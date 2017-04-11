// Package settings is for facilitating the usage of user settings, like themes.
package settings

import (
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// DefaultThemeID is the ID of the document that contains the variables for the
// default theme
const DefaultThemeID = consts.Settings + "-theme"

// Theme is a struct that contains all the values for the CSS variables used
// in the CSS called "theme.css". This stylesheet is ued by the client-side
// apps for having a common look that can be customized by the user.
type Theme struct {
	ThemeID  string `json:"_id,omitempty"`  // couchdb _id
	ThemeRev string `json:"_rev,omitempty"` // couchdb _rev
	Logo     string `json:"logo"`
	Base00   string `json:"base00"`
	Base01   string `json:"base01"`
	Base02   string `json:"base02"`
	Base03   string `json:"base03"`
	Base04   string `json:"base04"`
	Base05   string `json:"base05"`
	Base06   string `json:"base06"`
	Base07   string `json:"base07"`
	Base08   string `json:"base08"`
	Base09   string `json:"base09"`
	Base0A   string `json:"base0A"`
	Base0B   string `json:"base0B"`
	Base0C   string `json:"base0C"`
	Base0D   string `json:"base0D"`
	Base0E   string `json:"base0E"`
	Base0F   string `json:"base0F"`
}

// ID returns the theme qualified identifier
func (t *Theme) ID() string { return t.ThemeID }

// Rev returns the theme revision
func (t *Theme) Rev() string { return t.ThemeRev }

// DocType returns the theme document type
func (t *Theme) DocType() string { return consts.Settings }

// Clone returns a new theme with the same values
func (t *Theme) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

// SetID changes the theme qualified identifier
func (t *Theme) SetID(id string) { t.ThemeID = id }

// SetRev changes the theme revision
func (t *Theme) SetRev(rev string) { t.ThemeRev = rev }

// CreateDefaultTheme creates a theme that the user can customize later
func CreateDefaultTheme(db couchdb.Database) error {
	suffix := ""
	if config.IsDevRelease() {
		suffix = "-dev"
	}
	return couchdb.CreateNamedDocWithDB(db, &Theme{
		ThemeID: DefaultThemeID,
		Logo:    "/assets/images/cozy" + suffix + ".svg",
		Base00:  "#EAEEF2",
		Base01:  "#DDE6EF",
		Base02:  "#C8D5DF",
		Base03:  "#ACB8C5",
		Base04:  "#92A0B2",
		Base05:  "#748192",
		Base06:  "#4F5B69",
		Base07:  "#32363F",
		Base08:  "#FF3713",
		Base09:  "#FFAE5F",
		Base0A:  "#FFC643",
		Base0B:  "#16D943",
		Base0C:  "#4DCEC5",
		Base0D:  "#33A6FF",
		Base0E:  "#9169F2",
		Base0F:  "#EC7E63",
	})
}

// DefaultTheme return the document for the default theme
func DefaultTheme(db couchdb.Database) (*Theme, error) {
	theme := &Theme{}
	err := couchdb.GetDoc(db, consts.Settings, DefaultThemeID, theme)
	return theme, err
}

var (
	_ couchdb.Doc = &Theme{}
)
