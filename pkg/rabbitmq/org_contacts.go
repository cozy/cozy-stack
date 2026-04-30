package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/utils"
)

// SyncCreatedOrgContact syncs a newly created B2B user as an external contact
// into every other instance of the organization.
func SyncCreatedOrgContact(ctx context.Context, target *instance.Instance, msg UserCreatedMessage) error {
	email := strings.TrimSpace(msg.InternalEmail)
	if email == "" {
		return fmt.Errorf("user.created: missing internalEmail for organization contact sync")
	}
	name, err := target.SettingsPublicName()
	if err != nil {
		return fmt.Errorf("user.created: resolve contact name for %s: %w", target.Domain, err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("user.created: missing public_name in settings for %s", target.Domain)
	}
	targetURL := target.PageURL("", nil)

	instances, err := listOrgContactInstances("user.created", msg.OrganizationDomain)
	if err != nil {
		return err
	}

	var lastErr error
	for _, inst := range instances {
		if err := ctx.Err(); err != nil {
			return err
		}
		if inst.HasDomain(msg.WorkplaceFqdn) {
			log.Debugf("user.created: creating contacts for own instance %s for organization contact %s", inst.Domain, targetURL)
			if err := syncExistingOrgContactsToCreatedUser(ctx, target, instances, msg.WorkplaceFqdn); err != nil {
				log.Errorf("%v", err)
				lastErr = err
			}
			continue
		}

		existing, err := findExternalOrgContactByEmail(inst, email)
		if err != nil {
			wrappedErr := fmt.Errorf("user.created: find external contact for %s in %s: %w", email, inst.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}
		if existing != nil {
			log.Infof("user.created: external contact for %s already exists in %s, skipping", email, inst.Domain)
			continue
		}

		if _, err := contact.Create(inst, contact.CreateOptions{
			Email:    email,
			Name:     name,
			CozyURL:  targetURL,
			Phone:    msg.Mobile,
			External: true,
		}); err != nil {
			wrappedErr := fmt.Errorf("user.created: create contact for %s in %s: %w", email, inst.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}
		log.Infof("user.created: created organization contact for %s in %s", email, inst.Domain)
	}

	return lastErr
}

// SyncDeletedOrgContact removes a deleted B2B user from every other instance
// of the organization.
func SyncDeletedOrgContact(ctx context.Context, msg UserDeletedMessage) error {
	if strings.TrimSpace(msg.WorkplaceFqdn) == "" {
		return fmt.Errorf("user.deleted: missing workplaceFqdn")
	}
	email := strings.TrimSpace(msg.InternalEmail)
	if email == "" {
		return fmt.Errorf("user.deleted: missing internalEmail")
	}

	instances, err := listOrgContactInstances("user.deleted", msg.Domain)
	if err != nil {
		return err
	}

	var lastErr error
	for _, inst := range instances {
		if err := ctx.Err(); err != nil {
			return err
		}
		if inst.HasDomain(msg.WorkplaceFqdn) {
			log.Debugf("user.deleted: skipping own instance %s for organization contact %s", inst.Domain, msg.WorkplaceFqdn)
			continue
		}

		existing, err := findExternalOrgContactByEmail(inst, email)
		if err != nil {
			wrappedErr := fmt.Errorf("user.deleted: find external contact for %s in %s: %w", email, inst.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}
		if existing == nil {
			log.Infof("user.deleted: no external contact for %s in %s, skipping", email, inst.Domain)
			continue
		}
		if err := couchdb.DeleteDoc(inst, existing); err != nil {
			wrappedErr := fmt.Errorf("user.deleted: delete contact %s for %s in %s: %w", existing.ID(), email, inst.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}
		log.Infof("user.deleted: deleted organization contact for %s in %s", email, inst.Domain)
	}

	return lastErr
}

func listOrgContactInstances(eventName, orgDomain string) ([]*instance.Instance, error) {
	orgDomain = utils.NormalizeDomain(orgDomain)
	if orgDomain == "" {
		return nil, fmt.Errorf("%s: missing organization domain", eventName)
	}

	list, err := lifecycle.ListOrgInstances(orgDomain)
	if err != nil {
		return nil, fmt.Errorf("%s: list organization instances by domain %s: %w", eventName, orgDomain, err)
	}
	if len(list) == 0 {
		log.Infof("%s: no instances found for organization domain %s", eventName, orgDomain)
		return nil, fmt.Errorf("%s: organization has no instances", eventName)
	}
	return list, nil
}

func syncExistingOrgContactsToCreatedUser(ctx context.Context, target *instance.Instance, instances []*instance.Instance, workplaceFqdn string) error {
	var lastErr error
	for _, inst := range instances {
		if err := ctx.Err(); err != nil {
			return err
		}
		if inst.HasDomain(workplaceFqdn) {
			continue
		}

		opts, err := externalOrgContactFromInstance(inst)
		if err != nil {
			wrappedErr := fmt.Errorf("user.created: build existing user contact for %s: %w", inst.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}

		existing, err := findExternalOrgContactByEmail(target, opts.Email)
		if err != nil {
			wrappedErr := fmt.Errorf("user.created: find external contact for %s in %s: %w", opts.Email, target.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}
		if existing != nil {
			log.Infof("user.created: external contact for %s already exists in %s, skipping", opts.Email, target.Domain)
			continue
		}

		if _, err := contact.Create(target, opts); err != nil {
			wrappedErr := fmt.Errorf("user.created: create contact for %s in %s: %w", opts.Email, target.Domain, err)
			log.Errorf("%v", wrappedErr)
			lastErr = wrappedErr
			continue
		}
		log.Infof("user.created: created organization contact for %s in %s", opts.Email, target.Domain)
	}
	return lastErr
}

func externalOrgContactFromInstance(inst *instance.Instance) (contact.CreateOptions, error) {
	settings, err := inst.SettingsDocument()
	if err != nil {
		return contact.CreateOptions{}, err
	}

	email, _ := settings.M["email"].(string)
	email = strings.TrimSpace(email)
	if email == "" {
		return contact.CreateOptions{}, fmt.Errorf("missing email in settings")
	}

	name, _ := settings.M["public_name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return contact.CreateOptions{}, fmt.Errorf("missing public_name in settings")
	}

	phone, _ := settings.M["phone"].(string)

	return contact.CreateOptions{
		Email:    email,
		Name:     name,
		CozyURL:  inst.PageURL("", nil),
		Phone:    phone,
		External: true,
	}, nil
}

func findExternalOrgContactByEmail(inst *instance.Instance, email string) (*contact.Contact, error) {
	matches, err := contact.FindAllByEmail(inst, email)
	if errors.Is(err, contact.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var external []*contact.Contact
	for _, doc := range matches {
		if doc.IsExternal() {
			external = append(external, doc)
		}
	}
	if len(external) > 1 {
		return nil, fmt.Errorf("multiple external contacts found for email %s", email)
	}
	if len(external) == 1 {
		return external[0], nil
	}
	return nil, nil
}
