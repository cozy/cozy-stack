package sharings

import (
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

	// For each recipient we send the email and update the status if an error
	// occurred.
	recipientsStatus, err := s.RecStatus(instance)
	if err != nil {
		return err
	}

	errorOccurred := false
	for _, rs := range recipientsStatus {
		recipient := rs.recipient

		// Generate recipient specific OAuth query string.
		recipientOAuthQueryString, err := generateOAuthQueryString(s, recipient, instance.Scheme())
		if err != nil {
			errorOccurred = logErrorAndSetRecipientStatus(rs, err)
			continue
		}

		// Generate the base values of the email to send, common to all recipients:
		// the description and the sharer's public name.
		sharingMessage, err := generateMailMessage(s, recipient, &mailTemplateValues{
			RecipientName:    recipient.Email,
			SharerPublicName: sharerPublicName,
			Description:      desc,
			OAuthQueryString: recipientOAuthQueryString,
		})
		if err != nil {
			errorOccurred = logErrorAndSetRecipientStatus(rs, err)
			continue
		}

		// We ask to queue a new mail job.
		// The returned values (other than the error) are ignored because they
		// are of no use in this situation.
		// FI: they correspond to the job information and to a channel with
		// which we can check the advancement of said job.
		_, _, err = instance.JobsBroker().PushJob(&jobs.JobRequest{
			WorkerType: "sendmail",
			Options:    nil,
			Message:    sharingMessage,
		})
		if err != nil {
			errorOccurred = logErrorAndSetRecipientStatus(rs, err)
			continue
		}
	}

	if errorOccurred {
		return ErrMailCouldNotBeSent
	}

	return nil
}

// logErrorAndSetRecipientStatus will log an error in the stack and set the
// status of the impacted recipient to "error".
func logErrorAndSetRecipientStatus(rs *RecipientStatus, err error) bool {
	log.Error(`[Sharing] An error occurred while trying to send
        the email invitation`, err)
	rs.Status = consts.ErrorSharingStatus
	return true
}

// generateMailMessage will extract and compute the relevant information
// from the sharing to generate the mail we will send to the specified
// recipient.
func generateMailMessage(s *Sharing, r *Recipient,
	mailValues *mailTemplateValues) (*jobs.Message, error) {
	// The address of the recipient.
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
func generateOAuthQueryString(s *Sharing, r *Recipient, scheme string) (string, error) {

	// Check if an oauth client exists for the owner at the recipient's.
	if r.Client.ClientID == "" || len(r.Client.RedirectURIs) < 1 {
		return "", ErrNoOAuthClient
	}

	// Check if the recipient has an URL.
	if r.URL == "" {
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

	oAuthQuery := url.URL{
		Host: r.URL,
		Path: "/sharings/request",
	}

	// The link/button we put in the email has to have an http:// or https://
	// prefix, otherwise it cannot be open in the browser.
	if oAuthQuery.Scheme != "http" && oAuthQuery.Scheme != "https" {
		oAuthQuery.Scheme = scheme
	}

	// We use url.encode to safely escape the query parameters.
	mapParamOAuthQuery := url.Values{
		"client_id":     {r.Client.ClientID},
		"redirect_uri":  {r.Client.RedirectURIs[0]},
		"response_type": {"code"},
		"scope":         {permissionsScope},
		"sharing_type":  {s.SharingType},
		"state":         {s.SharingID},
	}
	oAuthQuery.RawQuery = mapParamOAuthQuery.Encode()

	return oAuthQuery.String(), nil
}
