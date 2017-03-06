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

// The skeleton of the mail we will send. The values between "{{ }}" will be
// filled through the `mailTemplateValues` structure.
const (
	mailTemplateEn = `
        <h2>Hey {{.RecipientName}}!</h2>
        <p>{{.SharerPublicName}} wants to share a document with you! You will only be able to view it.</p>

        <p>The description given is: {{.Description}}.</p>

        <form action="{{.OAuthQueryString}}">
            <input type="submit" value="Accept this sharing" />
        </form>
        </p>
    `

	mailTemplateFr = `
        <h2>Bonjour {{.RecipientName}} !</h2>
        <p>{{.SharerPublicName}} veut partager un document avec vous ! Vous pourrez seulement le consulter.</p>

        <p>La description associée est : {{.Description}}.</p>

        <form action="{{.OAuthQueryString}}">
            <input type="submit" value="Accepter le partage" />
        </form>
    `
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

	// Generate the base values of the email to send, common to all recipients:
	// the description and the sharer's public name.
	mailValues := &mailTemplateValues{}

	// We get the Couchdb document describing the instance to get the sharer's
	// public name.
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(instance, consts.Settings,
		consts.InstanceSettingsID, doc)
	if err != nil {
		return err
	}
	sharerPublicName, _ := doc.M["public_name"].(string)
	mailValues.SharerPublicName = sharerPublicName

	// Fill in the description.
	if s.Desc == "" {
		mailValues.Description = "[No description provided]"
	} else {
		mailValues.Description = s.Desc
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
		recipientOAuthQueryString, err := generateOAuthQueryString(s, recipient)
		if err != nil {
			errorOccurred = logErrorAndSetRecipientStatus(rs, err)
			continue
		}

		// Augment base values with recipient specific information.
		mailValues.RecipientName = recipient.Email
		mailValues.OAuthQueryString = recipientOAuthQueryString

		sharingMessage, err := generateMailMessage(s, recipient, mailValues)
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

	// We create the mail parts: its content.
	mailPartEn := workers.MailPart{
		Type: "text/html",
		Body: mailTemplateEn}

	mailPartFr := workers.MailPart{
		Type: "text/html",
		Body: mailTemplateFr}

	mailParts := []*workers.MailPart{&mailPartEn, &mailPartFr}

	// The address of the recipient.
	if r.Email == "" {
		return nil, ErrRecipientHasNoEmail
	}
	mailAddresses := []*workers.MailAddress{&workers.MailAddress{Name: r.Email,
		Email: r.Email}}

	mailOpts := workers.MailOptions{
		Mode:           "from",
		From:           nil, // Will be filled by the stack.
		To:             mailAddresses,
		Subject:        "New sharing request / Nouvelle demande de partage",
		Dialer:         nil, // Will be filled by the stack.
		Date:           nil, // Will be filled by the stack.
		Parts:          mailParts,
		TemplateValues: mailValues}

	message, err := jobs.NewMessage(jobs.JSONEncoding, mailOpts)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// generateOAuthQueryString takes care of creating a correct OAuth request for
// the given sharing and recipient.
func generateOAuthQueryString(s *Sharing, r *Recipient) (string, error) {

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

	// We use url.encode to safely escape the query string.
	mapParamQueryString := url.Values{}
	mapParamQueryString["client_id"] = []string{r.Client.ClientID}
	mapParamQueryString["redirect_uri"] = []string{r.Client.RedirectURIs[0]}
	mapParamQueryString["response_type"] = []string{"code"}
	mapParamQueryString["scope"] = []string{permissionsScope}
	mapParamQueryString["sharing_type"] = []string{s.SharingType}
	mapParamQueryString["state"] = []string{s.SharingID}

	paramQueryString := mapParamQueryString.Encode()

	queryString := fmt.Sprintf("%s/sharings/request?%s", r.URL,
		paramQueryString)

	return queryString, nil
}
