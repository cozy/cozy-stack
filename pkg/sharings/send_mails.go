package sharings

import (
	"fmt"
	"net/url"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/jobs/workers"
)

// The sharing-dependant information: the recipient's name, the sharer's public
// name, the description of the sharing, and the OAuth query string.
type mailTemplateValues struct {
	RecipientName    string
	SharerPublicName string
	Description      string
	OAuthQueryString string
}

// SendSharingMails will generate the mail containing the details
// regarding this sharing, and will then send it to all the recipients.
func SendSharingMails(instance *instance.Instance, s *Sharing) error {
	// We get the Couchdb document describing the instance to get the sharer's
	// public name.
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(instance, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return err
	}

	sharerPublicName, _ := doc.M["public_name"].(string)
	// Fill in the description.
	var desc string
	if s.Desc == "" {
		desc = "[No description provided]"
	} else {
		desc = s.Desc
	}

	errorOccurred := false
	for _, rs := range s.RecipientsStatus {
		// Sanity check: recipient is private.
		if rs.recipient == nil {
			rs.recipient, err = GetRecipient(instance, rs.RefRecipient.ID)
			if err != nil {
				return err
			}
		}

		// Generate recipient specific OAuth query string.
		oAuthStr, errOAuth := generateOAuthQueryString(s, rs, instance.Scheme())
		if errOAuth != nil {
			errorOccurred = logError(errOAuth)
			continue
		}

		// Generate the base values of the email to send, common to all
		// recipients: the description and the sharer's public name.
		sharingMessage, errGenMail := generateMailMessage(s, rs.recipient,
			&mailTemplateValues{
				RecipientName:    rs.recipient.Email,
				SharerPublicName: sharerPublicName,
				Description:      desc,
				OAuthQueryString: oAuthStr,
			},
		)
		if errGenMail != nil {
			errorOccurred = logError(errGenMail)
			continue
		}

		// We ask to queue a new mail job.
		// The returned values (other than the error) are ignored because they
		// are of no use in this situation.
		// FI: they correspond to the job information and to a channel with
		// which we can check the advancement of said job.
		_, _, errJobs := instance.JobsBroker().PushJob(&jobs.JobRequest{
			WorkerType: "sendmail",
			Options:    nil,
			Message:    sharingMessage,
		})
		if errJobs != nil {
			errorOccurred = logError(errJobs)
			continue
		}

		// Job was created, we set the status to "pending".
		rs.Status = consts.PendingSharingStatus
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
func logError(err error) bool {
	log.Error("[sharing] An error occurred while trying to send the email "+
		"invitation: ", err)
	return true
}

// generateMailMessage will extract and compute the relevant information
// from the sharing to generate the mail we will send to the specified
// recipient.
func generateMailMessage(s *Sharing, r *Recipient, mailValues *mailTemplateValues) (*jobs.Message, error) {
	if r.Email == "" {
		return nil, ErrRecipientHasNoEmail
	}
	mailAddresses := []*workers.MailAddress{&workers.MailAddress{
		Name:  r.Email,
		Email: r.Email,
	}}
	return jobs.NewMessage(jobs.JSONEncoding, workers.MailOptions{
		Mode:           "from",
		To:             mailAddresses,
		Subject:        "New sharing request / Nouvelle demande de partage",
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
	})
}

// generateOAuthQueryString takes care of creating a correct OAuth request for
// the given sharing and recipient.
func generateOAuthQueryString(s *Sharing, rs *RecipientStatus, scheme string) (string, error) {

	// Check if an oauth client exists for the owner at the recipient's.
	if rs.Client.ClientID == "" || len(rs.Client.RedirectURIs) < 1 {
		return "", ErrNoOAuthClient
	}

	// Check if the recipient has an URL.
	if rs.recipient.URL == "" {
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

	oAuthQuery, err := url.Parse(rs.recipient.URL)
	if err != nil {
		return "", err
	}
	// Special scenario: if r.URL doesn't have an "http://" or "https://" prefix
	// then `url.Parse` doesn't set any host.
	if oAuthQuery.Host == "" {
		oAuthQuery.Host = rs.recipient.URL
	}
	oAuthQuery.Path = "/sharings/request"
	// The link/button we put in the email has to have an http:// or https://
	// prefix, otherwise it cannot be open in the browser.
	if oAuthQuery.Scheme != "http" && oAuthQuery.Scheme != "https" {
		oAuthQuery.Scheme = scheme
	}

	mapParamOAuthQuery := url.Values{
		"client_id":     {rs.Client.ClientID},
		"redirect_uri":  {rs.Client.RedirectURIs[0]},
		"response_type": {"code"},
		"scope":         {permissionsScope},
		"sharing_type":  {s.SharingType},
		"state":         {s.SharingID},
	}
	oAuthQuery.RawQuery = mapParamOAuthQuery.Encode()

	return oAuthQuery.String(), nil
}
