package sharings

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/initials"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/hashicorp/go-multierror"
	"github.com/labstack/echo/v4"
)

// CreateSharing initializes a new sharing (on the sharer)
func CreateSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var s sharing.Sharing
	obj, err := jsonapi.Bind(c.Request().Body, &s)
	if err != nil {
		return jsonapi.BadJSON()
	}

	slug, err := checkCreatePermissions(c, &s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	if err = s.BeOwner(inst, slug); err != nil {
		return wrapErrors(err)
	}

	if rel, ok := obj.GetRelationship("recipients"); ok {
		if data, ok := rel.Data.([]interface{}); ok {
			for _, ref := range data {
				if id, ok := ref.(map[string]interface{})["id"].(string); ok {
					if err = s.AddContact(inst, id, false); err != nil {
						return err
					}
				}
			}
		}
	}

	if rel, ok := obj.GetRelationship("read_only_recipients"); ok {
		if data, ok := rel.Data.([]interface{}); ok {
			for _, ref := range data {
				if id, ok := ref.(map[string]interface{})["id"].(string); ok {
					if err = s.AddContact(inst, id, true); err != nil {
						return err
					}
				}
			}
		}
	}

	codes, err := s.Create(inst)
	if err != nil {
		return wrapErrors(err)
	}
	if err = s.SendInvitations(inst, codes); err != nil {
		return wrapErrors(err)
	}
	as := &sharing.APISharing{
		Sharing:     &s,
		Credentials: nil,
		SharedDocs:  nil,
	}
	return jsonapi.Data(c, http.StatusCreated, as, nil)
}

// PutSharing creates a sharing request (on the recipient's cozy)
func PutSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var s sharing.Sharing
	obj, err := jsonapi.Bind(c.Request().Body, &s)
	if err != nil {
		return jsonapi.BadJSON()
	}
	s.SID = obj.ID
	s.ShortcutID = ""

	if err := s.CreateRequest(inst); err != nil {
		return wrapErrors(err)
	}

	if c.QueryParam("shortcut") == "true" {
		u := c.QueryParam("url")
		if err := s.CreateShortcut(inst, u); err != nil {
			return wrapErrors(err)
		}
	}

	as := &sharing.APISharing{
		Sharing:     &s,
		Credentials: nil,
		SharedDocs:  nil,
	}
	return jsonapi.Data(c, http.StatusCreated, as, nil)
}

// jsonapiSharingWithDocs is an helper to send a JSON-API response for a
// sharing with its shared docs
func jsonapiSharingWithDocs(c echo.Context, s *sharing.Sharing) error {
	inst := middlewares.GetInstance(c)
	sharedDocs, err := sharing.GetSharedDocsBySharingIDs(inst, []string{s.SID})
	if err != nil {
		return wrapErrors(err)
	}
	docs := sharedDocs[s.SID]
	as := &sharing.APISharing{
		Sharing:     s,
		Credentials: nil,
		SharedDocs:  docs,
	}
	return jsonapi.Data(c, http.StatusOK, as, nil)
}

// GetSharing returns the sharing document associated to the given sharingID
// and which documents have been shared.
// The requester must have the permission on at least one doctype declared in
// the sharing document.
func GetSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	if err = checkGetPermissions(c, s); err != nil {
		return wrapErrors(err)
	}
	return jsonapiSharingWithDocs(c, s)
}

// GetSharingsInfoByDocType returns, for a given doctype, all the sharing
// information, i.e. the involved sharings and the shared documents
func GetSharingsInfoByDocType(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	docType := c.Param("doctype")

	sharings, err := sharing.GetSharingsByDocType(inst, docType)
	if err != nil {
		inst.Logger().WithField("nspace", "sharing").Errorf("GetSharingsByDocType error: %s", err)
		return wrapErrors(err)
	}
	if len(sharings) == 0 {
		return jsonapi.DataList(c, http.StatusOK, nil, nil)
	}
	sharingIDs := make([]string, len(sharings))
	i := 0
	for sID, s := range sharings {
		if err = checkGetPermissions(c, s); err != nil {
			return wrapErrors(err)
		}
		sharingIDs[i] = sID
		i++
	}
	sDocs, err := sharing.GetSharedDocsBySharingIDs(inst, sharingIDs)
	if err != nil {
		inst.Logger().WithField("nspace", "sharing").Errorf("GetSharedDocsBySharingIDs error: %s", err)
		return wrapErrors(err)
	}

	res := make([]*sharing.APISharing, len(sharings))
	i = 0
	for sID, s := range sharings {
		as := &sharing.APISharing{
			Sharing:     s,
			SharedDocs:  sDocs[sID],
			Credentials: nil,
		}
		res[i] = as
		i++
	}
	return sharing.InfoByDocTypeData(c, http.StatusOK, res)
}

// AnswerSharing is used to exchange credentials between 2 cozys, after the
// recipient has accepted a sharing.
func AnswerSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	var creds sharing.APICredentials
	if _, err = jsonapi.Bind(c.Request().Body, &creds); err != nil {
		return jsonapi.BadJSON()
	}
	ac, err := s.ProcessAnswer(inst, &creds)
	if err != nil {
		return wrapErrors(err)
	}
	return jsonapi.Data(c, http.StatusOK, ac, nil)
}

// Invite is used on a recipient to send them a mail inviation when the sharer
// only knows their instance, and not their email address.
func Invite(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	err := limits.CheckRateLimit(inst, limits.SharingInviteType)
	if limits.IsLimitReachedOrExceeded(err) {
		return wrapErrors(sharing.ErrInvitationNotSent)
	}
	var body sharing.InviteMsg
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return wrapErrors(err)
	}
	if body.Sharer == "" || body.Description == "" || body.Link == "" {
		return wrapErrors(sharing.ErrInvitationNotSent)
	}
	if err := sharing.SendInviteMail(inst, &body); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func addRecipientsToSharing(inst *instance.Instance, s *sharing.Sharing, rel *jsonapi.Relationship, readOnly bool) error {
	var err error
	if data, ok := rel.Data.([]interface{}); ok {
		ids := make(map[string]bool)
		for _, ref := range data {
			if id, ok := ref.(map[string]interface{})["id"].(string); ok {
				ids[id] = readOnly
			}
		}
		if s.Owner {
			err = s.AddContacts(inst, ids)
		} else {
			err = s.DelegateAddContacts(inst, ids)
		}
	}
	return err
}

// AddRecipients is used to add a member to a sharing
func AddRecipients(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	if _, err = checkCreatePermissions(c, s); err != nil {
		return wrapErrors(err)
	}
	var body sharing.Sharing
	obj, err := jsonapi.Bind(c.Request().Body, &body)
	if err != nil {
		return jsonapi.BadJSON()
	}
	if rel, ok := obj.GetRelationship("recipients"); ok {
		if err = addRecipientsToSharing(inst, s, rel, false); err != nil {
			return wrapErrors(err)
		}
	}
	if rel, ok := obj.GetRelationship("read_only_recipients"); ok {
		if err = addRecipientsToSharing(inst, s, rel, true); err != nil {
			return wrapErrors(err)
		}
	}
	return jsonapiSharingWithDocs(c, s)
}

// AddRecipientsDelegated is used to add a member to a sharing on the owner's cozy
// when it's the recipient's cozy that sends the mail invitation.
func AddRecipientsDelegated(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	if !s.Owner || !s.Open {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	var body sharing.Sharing
	obj, err := jsonapi.Bind(c.Request().Body, &body)
	if err != nil {
		return jsonapi.BadJSON()
	}
	states := make(map[string]string)
	if rel, ok := obj.GetRelationship("recipients"); ok {
		if data, ok := rel.Data.([]interface{}); ok {
			for _, ref := range data {
				contact, _ := ref.(map[string]interface{})
				email, _ := contact["email"].(string)
				cozy, _ := contact["instance"].(string)
				ro, _ := contact["read_only"].(bool)
				state := s.AddDelegatedContact(inst, email, cozy, ro)
				if email == "" {
					states[cozy] = state
				} else {
					states[email] = state
				}
			}
			if err := couchdb.UpdateDoc(inst, s); err != nil {
				return wrapErrors(err)
			}
			cloned := s.Clone().(*sharing.Sharing)
			go cloned.NotifyRecipients(inst, nil)
		}
	}
	return c.JSON(http.StatusOK, states)
}

// PutRecipients is used to update the members list on the recipients cozy
func PutRecipients(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	if s.Active {
		// If the sharing is active, we check the access token for a permission
		// on the sharing
		if err := hasSharingWritePermissions(c); err != nil {
			return err
		}
	} else {
		// If there is no synchronization, it means that we have a shortcut for
		// this sharing, and we can check the sharecode.
		token := middlewares.GetRequestToken(c)
		sharecode, err := s.GetSharecodeFromShortcut(inst)
		if err != nil || token != sharecode {
			return middlewares.ErrForbidden
		}
	}

	var body struct {
		Members []sharing.Member `json:"data"`
	}
	if err = json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return wrapErrors(err)
	}
	if err = s.UpdateRecipients(inst, body.Members); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func renderAlreadyAccepted(c echo.Context, inst *instance.Instance, cozyURL string) error {
	return c.Render(http.StatusBadRequest, "error.html", echo.Map{
		"Title":       inst.TemplateTitle(),
		"ThemeCSS":    middlewares.ThemeCSS(inst),
		"CozyUI":      middlewares.CozyUI(inst),
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"ErrorTitle":  "Error Sharing already accepted Title",
		"Error":       "Error Sharing already accepted",
		"Button":      inst.Translate("Error Sharing already accepted Button", cozyURL),
		"ButtonLink":  cozyURL,
		"Favicon":     middlewares.Favicon(inst),
	})
}

func renderDiscoveryForm(c echo.Context, inst *instance.Instance, code int, sharingID, state, sharecode string, m *sharing.Member) error {
	publicName, _ := inst.PublicName()
	fqdn := strings.TrimPrefix(m.Instance, "https://")
	slug, placeholder := "", consts.KnownFlatDomains[0]
	if context, ok := inst.SettingsContext(); ok {
		if domain, ok := context["sharing_domain"].(string); ok {
			placeholder = domain
		}
	}
	domain := placeholder
	if strings.HasPrefix(m.Instance, "http://") {
		slug, domain = m.Instance, ""
	} else if parts := strings.SplitN(fqdn, ".", 2); len(parts) == 2 {
		slug, domain = parts[0], parts[1]
	}
	return c.Render(code, "sharing_discovery.html", echo.Map{
		"Title":             inst.TemplateTitle(),
		"CozyUI":            middlewares.CozyUI(inst),
		"ThemeCSS":          middlewares.ThemeCSS(inst),
		"Domain":            inst.ContextualDomain(),
		"ContextName":       inst.ContextName,
		"Locale":            inst.Locale,
		"PublicName":        publicName,
		"RecipientName":     m.Name,
		"RecipientSlug":     slug,
		"RecipientDomain":   domain,
		"PlaceholderDomain": placeholder,
		"SharingID":         sharingID,
		"State":             state,
		"ShareCode":         sharecode,
		"URLError":          code == http.StatusBadRequest,
		"NotEmailError":     code == http.StatusPreconditionFailed,
		"Favicon":           middlewares.Favicon(inst),
	})
}

// GetDiscovery displays a form where a recipient can give the address of their
// cozy instance
func GetDiscovery(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	state := c.QueryParam("state")
	sharecode := c.FormValue("sharecode")

	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Title":       inst.TemplateTitle(),
			"ThemeCSS":    middlewares.ThemeCSS(inst),
			"CozyUI":      middlewares.CozyUI(inst),
			"Domain":      inst.ContextualDomain(),
			"ContextName": inst.ContextName,
			"Error":       "Error Invalid sharing",
			"Favicon":     middlewares.Favicon(inst),
		})
	}

	m := &sharing.Member{}
	if s.Owner {
		if sharecode != "" {
			m, err = s.FindMemberBySharecode(inst, sharecode)
		} else {
			m, err = s.FindMemberByState(state)
		}
		if err != nil || m.Status == sharing.MemberStatusRevoked {
			return c.Render(http.StatusBadRequest, "error.html", echo.Map{
				"Title":       inst.TemplateTitle(),
				"ThemeCSS":    middlewares.ThemeCSS(inst),
				"CozyUI":      middlewares.CozyUI(inst),
				"Domain":      inst.ContextualDomain(),
				"ContextName": inst.ContextName,
				"Error":       "Error Invalid sharing",
				"Favicon":     middlewares.Favicon(inst),
			})
		}
		if m.Status != sharing.MemberStatusMailNotSent &&
			m.Status != sharing.MemberStatusPendingInvitation &&
			m.Status != sharing.MemberStatusSeen {
			return renderAlreadyAccepted(c, inst, m.Instance)
		}
	}

	return renderDiscoveryForm(c, inst, http.StatusOK, sharingID, state, sharecode, m)
}

// PostDiscovery is called when the recipient has given its Cozy URL. Either an
// error is returned or the recipient will be redirected to their cozy.
//
// Note: we don't have an anti-CSRF system, we rely on shareCode being secret.
func PostDiscovery(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	state := c.FormValue("state")
	sharecode := c.FormValue("sharecode")
	cozyURL := c.FormValue("url")
	if cozyURL == "" {
		cozyURL = c.FormValue("slug")
	}
	cozyURL = strings.TrimSuffix(cozyURL, ".")
	if !strings.HasPrefix(cozyURL, "http://") && !strings.HasPrefix(cozyURL, "https://") {
		cozyURL = "https://" + cozyURL
	}
	if domain := c.FormValue("domain"); domain != "" && !strings.Contains(cozyURL, ".") {
		if domain == "mycosy.cloud" {
			domain = "mycozy.cloud"
		}
		cozyURL = cozyURL + "." + domain
	}

	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	var redirectURL, email string

	if s.Owner {
		var member *sharing.Member
		if sharecode != "" {
			member, err = s.FindMemberBySharecode(inst, sharecode)
			if err != nil {
				return wrapErrors(err)
			}
		} else {
			member, err = s.FindMemberByState(state)
			if err != nil {
				return wrapErrors(err)
			}
		}
		if strings.Contains(cozyURL, "@") {
			return renderDiscoveryForm(c, inst, http.StatusPreconditionFailed, sharingID, state, sharecode, member)
		}
		email = member.Email
		if err = s.RegisterCozyURL(inst, member, cozyURL); err != nil {
			if c.Request().Header.Get("Accept") == "application/json" {
				return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
			}
			if err == sharing.ErrAlreadyAccepted {
				return renderAlreadyAccepted(c, inst, cozyURL)
			}
			return renderDiscoveryForm(c, inst, http.StatusBadRequest, sharingID, state, sharecode, member)
		}
		redirectURL, err = member.GenerateOAuthURL(s)
		if err != nil {
			return wrapErrors(err)
		}
		sharing.PersistInstanceURL(inst, member.Email, member.Instance)
	} else {
		redirectURL, err = s.DelegateDiscovery(inst, state, cozyURL)
		if err != nil {
			if err == sharing.ErrInvalidURL {
				if c.Request().Header.Get("Accept") == "application/json" {
					return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
				}
				return renderDiscoveryForm(c, inst, http.StatusBadRequest, sharingID, state, sharecode, &sharing.Member{})
			}
			return wrapErrors(err)
		}
	}

	if c.Request().Header.Get("Accept") == "application/json" {
		m := echo.Map{"redirect": redirectURL}
		if email != "" {
			m["email"] = email
		}
		return c.JSON(http.StatusOK, m)
	}
	return c.Redirect(http.StatusFound, redirectURL)
}

// GetAvatar returns the avatar of the given member of the sharing.
func GetAvatar(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		return jsonapi.InvalidParameter("index", err)
	}
	if index > len(s.Members) {
		return jsonapi.NotFound(errors.New("member not found"))
	}
	m := s.Members[index]

	// Use the local avatar
	if m.Instance == "" || m.Instance == inst.PageURL("", nil) {
		img, mime, err := initials.Image(m.PublicName)
		if err != nil {
			return wrapErrors(err)
		}
		return c.Blob(http.StatusOK, mime, img)
	}

	// Use the public avatar from the member's instance
	res, err := http.Get(m.Instance + "/public/avatar?fallback=initials")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return c.Stream(res.StatusCode, res.Header.Get("Content-Type"), res.Body)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	// Create a sharing
	router.POST("/", CreateSharing)        // On the sharer
	router.PUT("/:sharing-id", PutSharing) // On a recipient
	router.GET("/:sharing-id", GetSharing)
	router.POST("/:sharing-id/answer", AnswerSharing)
	router.POST("/invite", Invite) // TODO Remove this route (cf TestRemoveTheDeprecatedInviteRoute)

	// Managing recipients
	router.POST("/:sharing-id/recipients", AddRecipients)
	router.PUT("/:sharing-id/recipients", PutRecipients)
	router.DELETE("/:sharing-id/recipients", RevokeSharing)                                                  // On the sharer
	router.DELETE("/:sharing-id/recipients/:index", RevokeRecipient)                                         // On the sharer
	router.POST("/:sharing-id/recipients/:index/readonly", AddReadOnly)                                      // On the sharer
	router.POST("/:sharing-id/recipients/self/readonly", DowngradeToReadOnly, checkSharingWritePermissions)  // On the recipient
	router.DELETE("/:sharing-id/recipients/:index/readonly", RemoveReadOnly)                                 // On the sharer
	router.DELETE("/:sharing-id/recipients/self/readonly", UpgradeToReadWrite, checkSharingWritePermissions) // On the recipient
	router.DELETE("/:sharing-id", RevocationRecipientNotif, checkSharingWritePermissions)                    // On the recipient
	router.DELETE("/:sharing-id/recipients/self", RevokeRecipientBySelf)                                     // On the recipient
	router.DELETE("/:sharing-id/answer", RevocationOwnerNotif, checkSharingWritePermissions)                 // On the sharer

	// Delegated routes for open sharing
	router.POST("/:sharing-id/recipients/delegated", AddRecipientsDelegated, checkSharingWritePermissions)

	// Misc
	router.GET("/doctype/:doctype", GetSharingsInfoByDocType)
	router.GET("/:sharing-id/recipients/:index/avatar", GetAvatar)

	// Register the URL of their Cozy for recipients
	router.GET("/:sharing-id/discovery", GetDiscovery)
	router.POST("/:sharing-id/discovery", PostDiscovery)

	// Replicator routes
	replicatorRoutes(router)
}

func extractSlugFromSourceID(sourceID string) (string, error) {
	parts := strings.SplitN(sourceID, "/", 2)
	if len(parts) < 2 {
		return "", jsonapi.BadRequest(errors.New("Invalid request"))
	}
	slug := parts[1]
	return slug, nil
}

// checkCreatePermissions checks the sharer's token has all the permissions
// matching the ones defined in the sharing document
func checkCreatePermissions(c echo.Context, s *sharing.Sharing) (string, error) {
	requestPerm, err := middlewares.GetPermission(c)
	if err != nil {
		return "", err
	}
	if requestPerm.Type != permission.TypeWebapp &&
		requestPerm.Type != permission.TypeOauth &&
		requestPerm.Type != permission.TypeCLI {
		return "", permission.ErrInvalidAudience
	}
	for _, r := range s.Rules {
		pr := permission.Rule{
			Title:    r.Title,
			Type:     r.DocType,
			Verbs:    permission.ALL,
			Selector: r.Selector,
			Values:   r.Values,
		}
		if !requestPerm.Permissions.RuleInSubset(pr) {
			return "", echo.NewHTTPError(http.StatusForbidden)
		}
	}
	if requestPerm.Type == permission.TypeOauth {
		if requestPerm.Client != nil {
			oauthClient := requestPerm.Client.(*oauth.Client)
			if slug := oauth.GetLinkedAppSlug(oauthClient.SoftwareID); slug != "" {
				return slug, nil
			}
		}
		return "", nil
	}
	return extractSlugFromSourceID(requestPerm.SourceID)
}

// checkGetPermissions checks the requester's token has at least one doctype
// permission declared in the rules of the sharing document
func checkGetPermissions(c echo.Context, s *sharing.Sharing) error {
	requestPerm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}

	if requestPerm.Type == permission.TypeSharePreview &&
		requestPerm.SourceID == consts.Sharings+"/"+s.SID {
		return nil
	}
	if requestPerm.Type != permission.TypeWebapp &&
		requestPerm.Type != permission.TypeOauth &&
		requestPerm.Type != permission.TypeCLI {
		return permission.ErrInvalidAudience
	}

	for _, r := range s.Rules {
		pr := permission.Rule{
			Title:    r.Title,
			Type:     r.DocType,
			Verbs:    permission.Verbs(permission.GET),
			Selector: r.Selector,
			Values:   r.Values,
		}
		if requestPerm.Permissions.RuleInSubset(pr) {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusForbidden)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	if merr, ok := err.(*multierror.Error); ok {
		err = merr.WrappedErrors()[0]
	}
	switch err {
	case contact.ErrNoMailAddress:
		return jsonapi.InvalidAttribute("recipients", err)
	case sharing.ErrNoRecipients, sharing.ErrNoRules:
		return jsonapi.BadRequest(err)
	case sharing.ErrInvalidURL:
		return jsonapi.InvalidParameter("url", err)
	case sharing.ErrInvalidSharing, sharing.ErrInvalidRule:
		return jsonapi.BadRequest(err)
	case sharing.ErrMemberNotFound:
		return jsonapi.NotFound(err)
	case sharing.ErrInvitationNotSent:
		return jsonapi.BadRequest(err)
	case sharing.ErrRequestFailed:
		return jsonapi.BadGateway(err)
	case sharing.ErrNoOAuthClient:
		return jsonapi.BadRequest(err)
	case sharing.ErrMissingID, sharing.ErrMissingRev:
		return jsonapi.BadRequest(err)
	case sharing.ErrInternalServerError:
		return jsonapi.InternalServerError(err)
	case sharing.ErrMissingFileMetadata:
		return jsonapi.NotFound(err)
	case sharing.ErrFolderNotFound:
		return jsonapi.NotFound(err)
	case sharing.ErrSafety:
		return jsonapi.BadRequest(err)
	case sharing.ErrAlreadyAccepted:
		return jsonapi.Conflict(err)
	case vfs.ErrInvalidHash:
		return jsonapi.InvalidParameter("md5sum", err)
	case vfs.ErrContentLengthMismatch:
		return jsonapi.PreconditionFailed("Content-Length", err)
	case vfs.ErrConflict:
		return jsonapi.Conflict(err)
	case vfs.ErrFileTooBig:
		return jsonapi.Errorf(http.StatusRequestEntityTooLarge, "%s", err)
	case permission.ErrExpiredToken:
		return jsonapi.BadRequest(err)
	}
	logger.WithNamespace("sharing").Warnf("Not wrapped error: %s", err)
	return err
}
