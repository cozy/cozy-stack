package apps

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/apps/registry"
	"github.com/cozy/cozy-stack/pkg/consts"
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

func registryListHandler(appType apps.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		t := "webapps"
		if appType == apps.Konnector {
			t = "konnectors"
		}
		doctype := "io.cozy.registry." + t
		if err := permissions.AllowWholeType(c, permissions.GET, doctype); err != nil {
			return err
		}
		apps := registry.All(t)
		objs := make([]jsonapi.Object, len(apps))
		for i, a := range apps {
			objs[i] = &registryApp{a}
		}
		return jsonapi.DataList(c, http.StatusOK, objs, nil)
	}
}

type registryVersion struct {
	*registry.Version
}

// MarshalJSON is part of the jsonapi.Object interface
func (v *registryVersion) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Version)
}

// Links is part of the jsonapi.Object interface
func (v *registryVersion) Links() *jsonapi.LinksList {
	return nil
}

// Relationships is part of the jsonapi.Object interface
func (v *registryVersion) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (v *registryVersion) Included() []jsonapi.Object {
	return []jsonapi.Object{}
}

// registryVersion is a jsonapi.Object
var _ jsonapi.Object = (*registryVersion)(nil)

func versionHandler(appType apps.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := permissions.AllowWholeType(c, permissions.GET, consts.Versions); err != nil {
			return err
		}
		name := c.Param("name")
		num := c.Param("version")
		v, err := registry.GetAppVersion(name, num)
		if err != nil {
			return wrapAppsError(err)
		}
		return jsonapi.Data(c, http.StatusOK, &registryVersion{v}, nil)
	}
}
