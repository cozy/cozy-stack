package intents

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// Service is a struct for an app that can serve an intent
type Service struct {
	Slug string `json:"slug"`
	Href string `json:"href"`
}

// Intent is a struct for a call from a client-side app to have another app do
// something for it
type Intent struct {
	ID          string    `json:"_id"`
	Rev         string    `json:"_rev,omitempty"`
	Action      string    `json:"action"`
	Type        string    `json:"type"`
	Permissions []string  `json:"permissions"`
	Client      string    `json:"client"`
	Services    []Service `json:"services"`
}

// GenerateHref creates the href where the service can be called for an intent
func (in *Intent) GenerateHref(instance *instance.Instance, slug, target string) string {
	u := instance.SubDomain(slug)
	if len(target) > 0 && target[0] == '/' {
		u.Path = ""
	}
	ret := u.String() + target
	if !strings.Contains(target, "{{") {
		target += "?intent={{.Intent}}"
	}
	tmpl := template.New("intent-" + target)
	if _, err := tmpl.Parse(target); err != nil {
		return ret
	}
	buf := new(bytes.Buffer)
	err := tmpl.Execute(buf, struct {
		Intent string
	}{in.ID})
	if err != nil {
		return ret
	}
	return u.String() + buf.String()
}

// FillServices looks at all the application that can answer this intent
// and save them in the services field
func (in *Intent) FillServices(instance *instance.Instance) error {
	var res []apps.Manifest
	err := couchdb.GetAllDocs(instance, consts.Apps, &couchdb.AllDocsRequest{}, &res)
	if err != nil {
		return err
	}
	for _, man := range res {
		if intent := man.FindIntent(in.Action, in.Type); intent != nil {
			href := in.GenerateHref(instance, man.Slug, intent.Href)
			service := Service{Slug: man.Slug, Href: href}
			in.Services = append(in.Services, service)
		}
	}
	return nil
}
