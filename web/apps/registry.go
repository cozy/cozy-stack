package apps

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/apps/registry"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
)

type registryApp struct {
	*registry.App
}

// MarshalJSON is part of the jsonapi.Object interface
func (a *registryApp) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.App)
}

// Links is part of the jsonapi.Object interface
func (a *registryApp) Links() *jsonapi.LinksList {
	return nil
}

// Relationships is part of the jsonapi.Object interface
func (a *registryApp) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (a *registryApp) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

// registryApp is a jsonapi.Object
var _ jsonapi.Object = (*registryApp)(nil)

func registryHandler(appType apps.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		t := "webapps"
		if appType == apps.Konnector {
			t = "konnectors"
		}
		doctype := "io.cozy.registries." + t
		if err := permissions.AllowWholeType(c, permissions.GET, doctype); err != nil {
			return err
		}
		apps := registry.All(appType)
		objs := make([]jsonapi.Object, len(apps))
		for i, a := range apps {
			objs[i] = &registryApp{a}
		}
		return jsonapi.DataList(c, http.StatusOK, objs, nil)
	}
}
