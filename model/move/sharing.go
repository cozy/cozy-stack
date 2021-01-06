package move

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	multierror "github.com/hashicorp/go-multierror"
)

// NotifySharings will notify other instances with a common sharing that this
// instance has moved, and will tell them to the new URL to use for the
// sharing.
func NotifySharings(inst *instance.Instance) error {
	// Let the dust settle a bit before starting the notifications
	time.Sleep(3 * time.Second)

	var sharings []*sharing.Sharing
	req := couchdb.AllDocsRequest{Limit: 1000}
	if err := couchdb.GetAllDocs(inst, consts.Sharings, &req, &sharings); err != nil {
		return err
	}

	var errm error
	for _, s := range sharings {
		if strings.HasPrefix(s.ID(), "_design") {
			continue
		}
		time.Sleep(100 * time.Millisecond)
		if err := notifySharing(inst, s); err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func notifySharing(inst *instance.Instance, s *sharing.Sharing) error {
	if !s.Owner {
		return notifyMember(inst, s, 0)
	}

	var errm error
	for i := range s.Members {
		if i == 0 {
			continue // skip the owner
		}
		if err := notifyMember(inst, s, i); err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func notifyMember(inst *instance.Instance, s *sharing.Sharing, index int) error {
	u, err := url.Parse(s.Members[index].Instance)
	if s.Members[index].Instance == "" || err != nil {
		return err
	}

	clientID := s.Credentials[0].InboundClientID
	if index > 0 {
		clientID = s.Credentials[index-1].InboundClientID
	}
	cli := &oauth.Client{ClientID: clientID}
	newToken, err := sharing.CreateAccessToken(inst, cli, s.ID(), permission.ALL)
	if err != nil {
		return err
	}
	moved := sharing.APIMoved{
		SharingID:    s.ID(),
		NewInstance:  inst.PageURL("", nil),
		AccessToken:  newToken.AccessToken,
		RefreshToken: newToken.RefreshToken,
	}
	data, err := jsonapi.MarshalObject(&moved)
	if err != nil {
		return err
	}
	body, err := json.Marshal(jsonapi.Document{Data: &data})
	if err != nil {
		return err
	}

	credIndex := 0
	if s.Owner {
		credIndex = index - 1
	}
	token := s.Credentials[credIndex].AccessToken.AccessToken

	opts := &request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/recipients/self/moved",
		Headers: request.Headers{
			"Accept":        "application/vnd.api+json",
			"Content-Type":  "application/vnd.api+json",
			"Authorization": "Bearer " + token,
		},
		Body:       bytes.NewReader(body),
		ParseError: sharing.ParseRequestError,
	}
	res, err := request.Req(opts)
	if res != nil && res.StatusCode/100 == 4 {
		res, err = sharing.RefreshToken(inst, err, s, &s.Members[index], &s.Credentials[credIndex], opts, body)
	}
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}
