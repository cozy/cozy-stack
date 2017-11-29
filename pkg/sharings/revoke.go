package sharings

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/labstack/echo"
)

// RevokeSharing revokes the sharing and deletes all the OAuth client and
// triggers associated with it.
//
// Revoking a sharing consists of setting the field `Revoked` to `true`.
// When the sharing is of type "two-way" both recipients and sharer have
// trigger(s) and OAuth client(s) to delete.
// In every other cases only the sharer has trigger(s) to delete and only the
// recipients have an OAuth client to delete.
//
// When this function is called it needs to call either `RevokerSharing` or
// `RevokeRecipient` depending on who initiated the revocation. This is
// represented by the `recursive` boolean parameter. The first call has this set
// to `true` while the subsequent call has it set to `false`.
func RevokeSharing(ins *instance.Instance, sharing *Sharing, recursive bool) error {
	var err error
	if sharing.Owner {
		for _, rs := range sharing.Recipients {
			if recursive {
				err = askToRevokeSharing(ins, sharing, &rs)
				if err != nil {
					continue
				}
			}

			if sharing.SharingType == consts.TwoWaySharing {
				err = deleteOAuthClient(ins, &rs)
				if err != nil {
					continue
				}
			}
		}

		err = removeSharingTriggers(ins, sharing.SID)
		if err != nil {
			return err
		}

	} else {
		if recursive {
			err = askToRevokeRecipient(ins, sharing, &sharing.Sharer)
			if err != nil {
				return err
			}
		}

		err = deleteOAuthClient(ins, &sharing.Sharer)
		if err != nil {
			return err
		}

		if sharing.SharingType == consts.TwoWaySharing {
			err = removeSharingTriggers(ins, sharing.SID)
			if err != nil {
				return err
			}
		}
	}
	sharing.Revoked = true
	ins.Logger().Debugf("[sharings] Setting status of sharing %s to revoked",
		sharing.SID)
	return couchdb.UpdateDoc(ins, sharing)
}

// RevokeRecipientByContactID revokes a recipient from the given sharing. Only the sharer
// can make this action.
func RevokeRecipientByContactID(ins *instance.Instance, sharing *Sharing, contactID string) error {
	if !sharing.Owner {
		return ErrOnlySharerCanRevokeRecipient
	}

	for _, rs := range sharing.Recipients {
		c := rs.Contact(ins)
		if c != nil && c.DocID == contactID {
			// TODO check how askToRevokeRecipient behave when no recipient has accepted the sharing
			if err := askToRevokeSharing(ins, sharing, &rs); err != nil {
				return err
			}
			return RevokeRecipient(ins, sharing, &rs)
		}
	}

	return nil
}

// RevokeRecipientByClientID revokes a recipient from the given sharing. Only the sharer
// can make this action.
func RevokeRecipientByClientID(ins *instance.Instance, sharing *Sharing, clientID string) error {
	if !sharing.Owner {
		return ErrOnlySharerCanRevokeRecipient
	}

	for _, rs := range sharing.Recipients {
		if rs.Client.ClientID == clientID {
			return RevokeRecipient(ins, sharing, &rs)
		}
	}

	return nil
}

// RevokeRecipient revokes a recipient from the given sharing. Only the sharer
// can make this action.
//
// If the sharing is of type "two-way" the sharer also has to remove the
// recipient's OAuth client.
//
// If there are no more recipients the sharing is revoked and the corresponding
// trigger is deleted.
func RevokeRecipient(ins *instance.Instance, sharing *Sharing, recipient *Member) error {
	if sharing.SharingType == consts.TwoWaySharing {
		if err := deleteOAuthClient(ins, recipient); err != nil {
			return err
		}
		recipient.InboundClientID = ""
	}

	recipient.Client = auth.Client{}
	recipient.AccessToken = auth.AccessToken{}
	recipient.Status = consts.SharingStatusRevoked

	toRevoke := true
	for _, recipient := range sharing.Recipients {
		if recipient.Status != consts.SharingStatusRevoked {
			toRevoke = false
		}
	}

	if toRevoke {
		// TODO check how removeSharingTriggers behave when no recipient has accepted the sharing
		if err := removeSharingTriggers(ins, sharing.SID); err != nil {
			ins.Logger().Errorf("[sharings] RevokeRecipient: Could not remove "+
				"triggers for sharing %s: %s", sharing.SID, err)
		}
		sharing.Revoked = true
	}

	return couchdb.UpdateDoc(ins, sharing)
}

func deleteOAuthClient(ins *instance.Instance, rs *Member) error {
	client, err := oauth.FindClient(ins, rs.InboundClientID)
	if err != nil {
		ins.Logger().Errorf("[sharings] deleteOAuthClient: Could not "+
			"find OAuth client %s: %s", rs.InboundClientID, err)
		return err
	}
	crErr := client.Delete(ins)
	if crErr != nil {
		ins.Logger().Errorf("[sharings] deleteOAuthClient: Could not "+
			"delete OAuth client %s: %s", rs.InboundClientID, err)
		return errors.New(crErr.Error)
	}

	ins.Logger().Debugf("[sharings] OAuth client %s deleted", rs.InboundClientID)
	rs.InboundClientID = ""
	return nil
}

func askToRevokeSharing(ins *instance.Instance, sharing *Sharing, rs *Member) error {
	return askToRevoke(ins, sharing, rs, "")
}

func askToRevokeRecipient(ins *instance.Instance, sharing *Sharing, rs *Member) error {
	// TODO: If the recipient revoke a one-way sharing, he  cannot request
	// the sharer yet, as he have no credentials
	if rs.RefContact.ID != "" {
		return askToRevoke(ins, sharing, rs, rs.Client.ClientID)
	}
	return nil

}

// TODO Once we will handle error properly (recipient is disconnected and
// what not) analyze the error returned and take proper actions every time this
// function is called.
func askToRevoke(ins *instance.Instance, sharing *Sharing, rs *Member, recipientClientID string) error {
	sharingID := sharing.SID
	c := rs.Contact(ins)
	if c == nil {
		ins.Logger().Errorf("[sharings] askToRevoke: Could not fetch "+
			"recipient %s from database", rs.RefContact.ID)
		return ErrRecipientDoesNotExist
	}

	var path string
	if recipientClientID == "" {
		path = fmt.Sprintf("/sharings/%s", sharingID)
	} else {
		// From the recipient point of view, only a two-way sharing
		// grants him the rights to request the sharer, as he doesn't have
		// any credentials otherwise.
		if sharing.SharingType == consts.TwoWaySharing {
			path = fmt.Sprintf("/sharings/%s/recipient/%s", sharingID,
				rs.InboundClientID)
		} else {
			return nil
		}
	}

	if rs.URL == "" {
		return ErrRecipientHasNoURL
	}
	u, err := url.Parse(rs.URL)
	if err != nil {
		return err
	}

	reqOpts := &request.Options{
		Domain:  u.Host,
		Scheme:  u.Scheme,
		Path:    path,
		Method:  http.MethodDelete,
		Queries: url.Values{consts.QueryParamRecursive: {"false"}},
		Headers: request.Headers{
			echo.HeaderAuthorization: fmt.Sprintf("Bearer %s",
				rs.AccessToken.AccessToken),
		},
		Body:       nil,
		NoResponse: true,
	}

	_, err = request.Req(reqOpts)

	if err != nil {
		if IsAuthError(err) {
			recInfo, errInfo := ExtractRecipientInfo(rs)
			if errInfo != nil {
				return errInfo
			}
			_, err = RefreshTokenAndRetry(ins, sharingID, recInfo, reqOpts)
		}
		if err != nil {
			ins.Logger().Errorf("[sharings] askToRevoke: Could not ask recipient "+
				"%s to revoke sharing %s: %v", rs.contact.Cozy[0].URL, sharingID, err)
		}
		return err
	}

	rs.Client = auth.Client{}
	rs.AccessToken = auth.AccessToken{}
	return nil
}

// RefreshTokenAndRetry is called after an authentication failure.
// It tries to renew the access_token and request again
func RefreshTokenAndRetry(ins *instance.Instance, sharingID string, rec *RecipientInfo, opts *request.Options) (*http.Response, error) {
	ins.Logger().Errorf("[sharing] The request is not authorized. "+
		"Trying to renew the token for %v", rec.Domain)

	req := &auth.Request{
		Domain:     opts.Domain,
		Scheme:     opts.Scheme,
		HTTPClient: new(http.Client),
	}
	sharing, recStatus, err := FindSharingRecipient(ins, sharingID, rec.Client.ClientID)
	if err != nil {
		return nil, err
	}
	refreshToken := rec.AccessToken.RefreshToken
	access, err := req.RefreshToken(&rec.Client, &rec.AccessToken)
	if err != nil {
		ins.Logger().Errorf("[sharing] Refresh token request failed: %v", err)
		return nil, err
	}
	access.RefreshToken = refreshToken
	recStatus.AccessToken = *access
	if err = couchdb.UpdateDoc(ins, sharing); err != nil {
		return nil, err
	}
	opts.Headers["Authorization"] = "Bearer " + access.AccessToken
	res, err := request.Req(opts)
	return res, err
}

// IsAuthError returns true if the given error is an authentication one
func IsAuthError(err error) bool {
	if v, ok := err.(*request.Error); ok {
		return v.Title == "Bad Request" || v.Title == "Unauthorized"
	}
	return false
}
