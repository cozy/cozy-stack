package sharings

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// RecipientInfo describes the recipient information that will be transmitted to
// the sharing workers.
type RecipientInfo struct {
	Domain      string
	Scheme      string
	Client      auth.Client
	AccessToken auth.AccessToken
}

// ExtractRecipientInfo returns a RecipientInfo from a Member
func ExtractRecipientInfo(m *Member) (*RecipientInfo, error) {
	if m.URL == "" {
		return nil, ErrRecipientHasNoURL
	}
	u, err := url.Parse(m.URL)
	if err != nil {
		return nil, err
	}
	info := RecipientInfo{
		Domain:      u.Host,
		Scheme:      u.Scheme,
		AccessToken: m.AccessToken,
		Client:      m.Client,
	}
	return &info, nil
}

// RefreshTokenAndRetry is called after an authentication failure.
// It tries to renew the access_token and request again
func RefreshTokenAndRetry(ins *instance.Instance, sharingID string, info *RecipientInfo, opts *request.Options) (*http.Response, error) {
	req := &auth.Request{
		Domain: opts.Domain,
		Scheme: opts.Scheme,
	}
	sharing, err := FindSharing(ins, sharingID)
	if err != nil {
		return nil, err
	}
	var m *Member
	if sharing.Owner {
		m, err = sharing.GetMemberFromClientID(ins, info.Client.ClientID)
		if err != nil {
			return nil, err
		}
	} else {
		if sharing.Sharer.Client.ClientID != info.Client.ClientID {
			return nil, ErrRecipientDoesNotExist
		}
		m = sharing.Sharer
	}
	refreshToken := info.AccessToken.RefreshToken
	access, err := req.RefreshToken(&info.Client, &info.AccessToken)
	if err != nil {
		ins.Logger().Errorf("[sharing] Refresh token request failed: %v", err)
		return nil, err
	}
	access.RefreshToken = refreshToken
	m.AccessToken = *access
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
