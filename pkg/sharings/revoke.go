package sharings

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/labstack/echo"
)

// RevokeSharing revokes the sharing and deletes all the OAuth client and
// triggers associated with it.
//
// Revoking a sharing consists of marking the permissions revoked.
// When the sharing is of type "two-way" both recipients and
// sharer have trigger(s) and OAuth client(s) to delete. In every other cases
// only the sharer has trigger(s) to delete and only the recipients have an
// OAuth client to delete.
func RevokeSharing(ins *instance.Instance, sharing *Sharing, recursive bool) error {
	if perms, err := sharing.Permissions(ins); err == nil {
		if err = expirePermissions(ins, perms); err != nil {
			return err
		}
	}

	if sharing.Owner {
		for _, rs := range sharing.Recipients {
			if recursive {
				if err := askToRevokeSharing(ins, sharing, &rs); err != nil {
					ins.Logger().Infof("[sharings] Can't revoke a recipient for %s: %s", sharing.SID, err)
					continue
				}
			}

			if sharing.SharingType == consts.TwoWaySharing {
				if err := DeleteOAuthClient(ins, &rs); err != nil {
					continue
				}
			}
		}

		if err := removeSharingTriggers(ins, sharing.SID); err != nil {
			return err
		}

	} else {
		if recursive {
			if err := askToRevokeRecipient(ins, sharing, sharing.Sharer); err != nil {
				ins.Logger().Infof("[sharings] Can't revoke the sharer for %s: %s", sharing.SID, err)
			}
		}

		_ = DeleteOAuthClient(ins, sharing.Sharer)

		if sharing.SharingType == consts.TwoWaySharing {
			if err := removeSharingTriggers(ins, sharing.SID); err != nil {
				return err
			}
		}
	}

	ins.Logger().Debugf("[sharings] Setting status of sharing %s to revoked", sharing.SID)
	sharing.UpdatedAt = time.Now()
	return couchdb.UpdateDoc(ins, sharing)
}

// RevokeRecipientByContactID revokes a recipient from the given sharing. Only the sharer
// can make this action.
func RevokeRecipientByContactID(ins *instance.Instance, sharing *Sharing, contactID string) error {
	if !sharing.Owner {
		return ErrOnlySharerCanRevokeRecipient
	}

	for i := range sharing.Recipients {
		rs := &sharing.Recipients[i]
		c := rs.Contact(ins)
		if c != nil && c.DocID == contactID {
			// TODO check how askToRevokeRecipient behave when no recipient has accepted the sharing
			if err := askToRevokeSharing(ins, sharing, rs); err != nil {
				return err
			}
			return RevokeRecipient(ins, sharing, rs)
		}
	}

	return nil
}

// RevokeRecipientByClientID revokes a recipient from the given sharing on the
// Cozy of the sharer. The recipient has already revoked the sharing on its
// Cozy.
func RevokeRecipientByClientID(ins *instance.Instance, sharing *Sharing, clientID string) error {
	if !sharing.Owner {
		return ErrOnlySharerCanRevokeRecipient
	}

	for i := range sharing.Recipients {
		rs := &sharing.Recipients[i]
		if rs.Client.ClientID == clientID {
			return RevokeRecipient(ins, sharing, rs)
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
		_ = DeleteOAuthClient(ins, recipient)
	}

	recipient.Client = auth.Client{}
	recipient.AccessToken = auth.AccessToken{}
	recipient.Status = consts.SharingStatusRevoked

	noActiveRecipients := true
	for _, recipient := range sharing.Recipients {
		if recipient.Status != consts.SharingStatusRevoked {
			noActiveRecipients = false
		}
	}

	if noActiveRecipients {
		// TODO check how removeSharingTriggers behave when no recipient has accepted the sharing
		if err := removeSharingTriggers(ins, sharing.SID); err != nil {
			ins.Logger().Errorf("[sharings] RevokeRecipient: Could not remove "+
				"triggers for sharing %s: %s", sharing.SID, err)
		}
	}

	sharing.UpdatedAt = time.Now()
	return couchdb.UpdateDoc(ins, sharing)
}

func expirePermissions(i *instance.Instance, perms *permissions.Permission) error {
	perms.ExpiresAt = time.Now()
	return couchdb.UpdateDoc(i, perms)
}

// DeleteOAuthClient deletes the OAuth client used by the member for
// authentifying its requests to this cozy.
func DeleteOAuthClient(ins *instance.Instance, rs *Member) error {
	if rs.InboundClientID == "" {
		return nil
	}
	client, err := oauth.FindClient(ins, rs.InboundClientID)
	if err != nil {
		ins.Logger().Errorf("[sharings] DeleteOAuthClient: Could not "+
			"find OAuth client %s: %s", rs.InboundClientID, err)
		return err
	}
	if crErr := client.Delete(ins); crErr != nil {
		ins.Logger().Errorf("[sharings] DeleteOAuthClient: Could not "+
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
	// TODO: If the recipient revoke a one-way sharing, he cannot request
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
			path = fmt.Sprintf("/sharings/%s/%s", sharingID, rs.InboundClientID)
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
