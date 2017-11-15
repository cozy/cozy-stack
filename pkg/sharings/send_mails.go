package sharings

import (
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
)

// The sharing-dependant information: the recipient's name, the sharer's public
// name, the description of the sharing, and the OAuth query string.
type mailTemplateValues struct {
	RecipientName    string
	SharerPublicName string
	Description      string
	SharingLink      string
}

// SendDiscoveryMail send a mail to the recipient, in order for him to give his
// URL to the sender
func SendDiscoveryMail(instance *instance.Instance, s *Sharing, rs *RecipientStatus) error {
	sharerPublicName, err := instance.PublicName()
	if err != nil {
		return err
	}
	// Fill in the description.
	desc := s.Description
	if desc == "" {
		desc = "[No description provided]"
	}
	discoveryLink, err := generateDiscoveryLink(instance, s, rs)
	if err != nil {
		return err
	}

	// Generate the base values of the email to send
	discoveryMsg, err := generateMailMessage(s, rs.recipient,
		&mailTemplateValues{
			RecipientName:    rs.recipient.Email[0].Address,
			SharerPublicName: sharerPublicName,
			Description:      desc,
			SharingLink:      discoveryLink,
		},
	)
	if err != nil {
		return err
	}
	_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     instance.Domain,
		WorkerType: "sendmail",
		Options:    nil,
		Message:    discoveryMsg,
	})
	return err
}

// SendSharingMails will generate the mail containing the details
// regarding this sharing, and will then send it to all the recipients.
func SendSharingMails(instance *instance.Instance, s *Sharing) error {
	sharerPublicName, err := instance.PublicName()
	if err != nil {
		return err
	}
	// Fill in the description.
	desc := s.Description
	if desc == "" {
		desc = "[No description provided]"
	}

	errorOccurred := false
	for _, rs := range s.RecipientsStatus {
		err = rs.GetRecipient(instance)
		if err != nil {
			return err
		}
		if len(rs.recipient.Email) == 0 {
			errorOccurred = logError(instance, ErrRecipientHasNoEmail)
			continue
		}
		// Special case if the recipient's URL is not known: start discovery
		if len(rs.recipient.Cozy) == 0 {
			err = SendDiscoveryMail(instance, s, rs)
			if err != nil {
				logError(instance, err)
				rs.Status = consts.SharingStatusMailNotSent
			} else {
				rs.Status = consts.SharingStatusPending
			}
			err = couchdb.UpdateDoc(instance, s)
			if err != nil {
				errorOccurred = logError(instance, err)
			}
			continue
		}
		// Send mail based on the recipient status
		if rs.Status == consts.SharingStatusMailNotSent {
			// Generate recipient specific OAuth query string.
			oAuthStr, errOAuth := GenerateOAuthQueryString(s, rs, instance.Scheme())
			if errOAuth != nil {
				errorOccurred = logError(instance, errOAuth)
				continue
			}

			// Generate the base values of the email to send, common to all
			// recipients: the description and the sharer's public name.
			sharingMessage, errGenMail := generateMailMessage(s, rs.recipient,
				&mailTemplateValues{
					RecipientName:    rs.recipient.Email[0].Address,
					SharerPublicName: sharerPublicName,
					Description:      desc,
					SharingLink:      oAuthStr,
				},
			)
			if errGenMail != nil {
				errorOccurred = logError(instance, errGenMail)
				continue
			}

			// We ask to queue a new mail job.
			// The returned values (other than the error) are ignored because they
			// are of no use in this situation.
			// FI: they correspond to the job information and to a channel with
			// which we can check the advancement of said job.
			_, errJobs := globals.GetBroker().PushJob(&jobs.JobRequest{
				Domain:     instance.Domain,
				WorkerType: "sendmail",
				Options:    nil,
				Message:    sharingMessage,
			})
			if errJobs != nil {
				errorOccurred = logError(instance, errJobs)
				continue
			}

			// Job was created, we set the status to "pending".
			rs.Status = consts.SharingStatusPending
		}
	}
	// Persist the modifications in the database.
	err = couchdb.UpdateDoc(instance, s)
	if err != nil {
		if errorOccurred {
			return fmt.Errorf("[sharing] Error updating the document (%v) "+
				"and sending the email invitation (%v)", err,
				ErrMailCouldNotBeSent)
		}
		return err
	}

	if errorOccurred {
		return ErrMailCouldNotBeSent
	}

	return nil
}

// logError will log an error in the stack.
func logError(i *instance.Instance, err error) bool {
	i.Logger().Error("[sharing] An error occurred while trying to send the email "+
		"invitation: ", err)
	return true
}

// generateMailMessage will extract and compute the relevant information
// from the sharing to generate the mail we will send to the specified
// recipient.
func generateMailMessage(s *Sharing, r *contacts.Contact, mailValues *mailTemplateValues) (jobs.Message, error) {
	if len(r.Email) == 0 {
		return nil, ErrRecipientHasNoEmail
	}
	mailAddresses := []*mails.Address{&mails.Address{
		Name:  r.Email[0].Address,
		Email: r.Email[0].Address,
	}}
	return jobs.NewMessage(mails.Options{
		Mode:           "from",
		To:             mailAddresses,
		Subject:        "New sharing request / Nouvelle demande de partage",
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
	})
}

// GenerateOAuthQueryString takes care of creating a correct OAuth request for
// the given sharing and recipient.
func GenerateOAuthQueryString(s *Sharing, rs *RecipientStatus, scheme string) (string, error) {

	// Check if an oauth client exists for the owner at the recipient's.
	if rs.Client.ClientID == "" || len(rs.Client.RedirectURIs) < 1 {
		return "", ErrNoOAuthClient
	}

	// Check if the recipient has an URL.
	if len(rs.recipient.Cozy) == 0 {
		return "", ErrRecipientHasNoURL
	}

	// In the sharing document the permissions are stored as a
	// `permissions.Set`. We need to convert them in a proper format to be able
	// to incorporate them in the OAuth query string.
	//
	// Optimization: the next four lines of code could be outside of this
	// function and also outside of the for loop that iterates on the
	// recipients in `SendSharingMails`.
	// I found it was clearer to leave it here, at the price of being less
	// optimized.
	permissionsScope, err := s.Permissions.MarshalScopeString()
	if err != nil {
		return "", err
	}

	oAuthQuery, err := url.Parse(rs.recipient.Cozy[0].URL)
	if err != nil {
		return "", err
	}
	// Special scenario: if r.URL doesn't have an "http://" or "https://" prefix
	// then `url.Parse` doesn't set any host.
	if oAuthQuery.Host == "" {
		oAuthQuery.Host = rs.recipient.Cozy[0].URL
	}
	oAuthQuery.Path = "/sharings/request"
	// The link/button we put in the email has to have an http:// or https://
	// prefix, otherwise it cannot be open in the browser.
	if oAuthQuery.Scheme != "http" && oAuthQuery.Scheme != "https" {
		oAuthQuery.Scheme = scheme
	}

	mapParamOAuthQuery := url.Values{
		consts.QueryParamAppSlug: {s.AppSlug},
		"client_id":              {rs.Client.ClientID},
		"redirect_uri":           {rs.Client.RedirectURIs[0]},
		"response_type":          {"code"},
		"scope":                  {permissionsScope},
		"sharing_type":           {s.SharingType},
		"state":                  {s.SharingID},
	}
	oAuthQuery.RawQuery = mapParamOAuthQuery.Encode()

	return oAuthQuery.String(), nil
}

func generateDiscoveryLink(instance *instance.Instance, s *Sharing, rs *RecipientStatus) (string, error) {
	// Check if the recipient has an URL.
	if len(rs.recipient.Email) == 0 {
		return "", ErrRecipientHasNoEmail
	}

	path := "/sharings/discovery"
	discQuery := url.Values{
		"recipient_id":    {rs.recipient.ID()},
		"sharing_id":      {s.SharingID},
		"recipient_email": {rs.recipient.Email[0].Address},
	}
	discURL := url.URL{
		Scheme:   instance.Scheme(),
		Host:     instance.Domain,
		Path:     path,
		RawQuery: discQuery.Encode(),
	}

	return discURL.String(), nil
}
