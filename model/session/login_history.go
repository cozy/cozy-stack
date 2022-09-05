package session

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo/v4"
	"github.com/mssola/user_agent"
	maxminddb "github.com/oschwald/maxminddb-golang"
)

// LoginEntry stores informations associated with a new login. It is useful to
// provide the user with informations about the history of all the logins that
// may have happened on its domain.
type LoginEntry struct {
	DocID       string `json:"_id,omitempty"`
	DocRev      string `json:"_rev,omitempty"`
	SessionID   string `json:"session_id"`
	IP          string `json:"ip"`
	City        string `json:"city,omitempty"`
	Subdivision string `json:"subdivision,omitempty"`
	Country     string `json:"country,omitempty"`
	// XXX No omitempty on os and browser, because they are indexed in couchdb
	UA                 string    `json:"user_agent"`
	OS                 string    `json:"os"`
	Browser            string    `json:"browser"`
	ClientRegistration bool      `json:"client_registration"`
	CreatedAt          time.Time `json:"created_at"`
}

// DocType implements couchdb.Doc
func (l *LoginEntry) DocType() string { return consts.SessionsLogins }

// ID implements couchdb.Doc
func (l *LoginEntry) ID() string { return l.DocID }

// SetID implements couchdb.Doc
func (l *LoginEntry) SetID(v string) { l.DocID = v }

// Rev implements couchdb.Doc
func (l *LoginEntry) Rev() string { return l.DocRev }

// SetRev implements couchdb.Doc
func (l *LoginEntry) SetRev(v string) { l.DocRev = v }

// Clone implements couchdb.Doc
func (l *LoginEntry) Clone() couchdb.Doc {
	clone := *l
	return &clone
}

func lookupIP(ip, locale string) (city, subdivision, country, timezone string) {
	geodb := config.GetConfig().GeoDB
	if geodb == "" {
		return
	}
	db, err := maxminddb.Open(geodb)
	if err != nil {
		logger.WithNamespace("sessions").Errorf("cannot open the geodb: %s", err)
		return
	}
	defer db.Close()

	var record struct {
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		Country struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		Location struct {
			TimeZone string `maxminddb:"time_zone"`
		} `maxminddb:"location"`
	}

	err = db.Lookup(net.ParseIP(ip), &record)
	if err != nil {
		logger.WithNamespace("sessions").Infof("cannot lookup %s: %s", ip, err)
		return
	}
	if c, ok := record.City.Names[locale]; ok {
		city = c
	} else if c, ok := record.City.Names["en"]; ok {
		city = c
	}
	if len(record.Subdivisions) > 0 {
		if s, ok := record.Subdivisions[0].Names[locale]; ok {
			subdivision = s
		} else if s, ok := record.Subdivisions[0].Names["en"]; ok {
			city = s
		}
	}
	if c, ok := record.Country.Names[locale]; ok {
		country = c
	} else if c, ok := record.Country.Names["en"]; ok {
		country = c
	}
	timezone = record.Location.TimeZone
	return
}

// StoreNewLoginEntry creates a new login entry in the database associated with
// the given instance.
func StoreNewLoginEntry(i *instance.Instance, sessionID, clientID string,
	req *http.Request, logMessage string, notifEnabled bool,
) error {
	var ip string
	if forwardedFor := req.Header.Get(echo.HeaderXForwardedFor); forwardedFor != "" {
		ip = strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
	}
	if ip == "" {
		ip = strings.Split(req.RemoteAddr, ":")[0]
	}

	city, subdivision, country, timezone := lookupIP(ip, i.Locale)
	rawUserAgent := req.UserAgent()
	ua := user_agent.New(rawUserAgent)
	os := ua.OS()
	browser, _ := ua.Browser()
	if strings.Contains(rawUserAgent, "CozyDrive") {
		browser = "CozyDrive"
	}

	createdAt := time.Now()
	i.Logger().WithNamespace("loginaudit").
		Infof("New connection from %s at %s (%s)", ip, createdAt, logMessage)
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			createdAt = createdAt.In(loc)
		}
	}

	l := &LoginEntry{
		IP:                 ip,
		SessionID:          sessionID,
		City:               city,
		Subdivision:        subdivision,
		Country:            country,
		UA:                 req.UserAgent(),
		OS:                 os,
		Browser:            browser,
		ClientRegistration: clientID != "",
		CreatedAt:          createdAt,
	}

	if err := couchdb.CreateDoc(i, l); err != nil {
		return err
	}

	if clientID != "" {
		if err := PushLoginRegistration(i, l, clientID); err != nil {
			i.Logger().Errorf("Could not push login in registration queue: %s", err)
		}
	} else if notifEnabled {
		if err := sendLoginNotification(i, l); err != nil {
			i.Logger().Errorf("Could not send login notification: %s", err)
		}
	}

	return nil
}

func sendLoginNotification(i *instance.Instance, l *LoginEntry) error {
	// Don't send a notification the first time the user logs in their Cozy, as
	// it doesn't make sense for the user. In general, this function is not
	// even called when this is the case, but sometimes the user can create
	// their Cozy from the manager with an OIDC flow, with no confirmation mail
	// no password choosing, and we need this trick for them.
	nb, _ := couchdb.CountNormalDocs(i, consts.SessionsLogins)
	if nb == 1 {
		return nil
	}

	var results []*LoginEntry
	r := &couchdb.FindRequest{
		UseIndex: "by-os-browser-ip",
		Selector: mango.And(
			mango.Equal("os", l.OS),
			mango.Equal("browser", l.Browser),
			mango.Equal("ip", l.IP),
			mango.NotEqual("_id", l.ID()),
		),
		Limit: 1,
	}
	err := couchdb.FindDocs(i, consts.SessionsLogins, r, &results)
	sendNotification := err != nil || len(results) == 0
	if !sendNotification {
		return nil
	}

	var changePassphraseLink string
	if i.IsPasswordAuthenticationEnabled() {
		changePassphraseLink = i.ChangePasswordURL()
	}
	var activateTwoFALink string
	if !i.HasAuthMode(instance.TwoFactorMail) {
		settingsURL := i.SubDomain(consts.SettingsSlug)
		settingsURL.Fragment = "/profile"
		activateTwoFALink = settingsURL.String()
	}

	layout := i.Translate("Time Format Long")
	time := i18n.LocalizeTime(l.CreatedAt, i.Locale, layout)

	templateValues := map[string]interface{}{
		"Time":                 time,
		"Country":              l.Country,
		"IP":                   l.IP,
		"Browser":              l.Browser,
		"OS":                   l.OS,
		"ChangePassphraseLink": changePassphraseLink,
		"ActivateTwoFALink":    activateTwoFALink,
	}

	return lifecycle.SendMail(i, &lifecycle.Mail{
		TemplateName:   "new_connection",
		TemplateValues: templateValues,
	})
}

// SendNewRegistrationNotification is used to send a notification to the user
// when a new OAuth client is registered.
func SendNewRegistrationNotification(i *instance.Instance, clientRegistrationID string) error {
	devicesLink := i.SubDomain(consts.SettingsSlug)
	devicesLink.Fragment = "/connectedDevices"
	revokeLink := i.SubDomain(consts.SettingsSlug)
	revokeLink.Fragment = "/connectedDevices/" + url.PathEscape(clientRegistrationID)
	templateValues := map[string]interface{}{
		"DevicesLink": devicesLink.String(),
		"RevokeLink":  revokeLink.String(),
	}

	return lifecycle.SendMail(i, &lifecycle.Mail{
		TemplateName:   "new_registration",
		TemplateValues: templateValues,
	})
}
