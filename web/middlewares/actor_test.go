package middlewares

import (
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestResolveRequestActor(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	inst := &instance.Instance{Domain: "alice.cozy.local"}

	t.Run("AnonymousShare", func(t *testing.T) {
		err := ResolveRequestActor(c, inst, &permission.Permission{Type: permission.TypeShareByLink})
		require.NoError(t, err)

		actor, ok := GetActor(c)
		require.True(t, ok)
		require.Equal(t, vfs.TrashedByKindAnonymousShare, actor.Kind)
		require.Empty(t, actor.DisplayName)
		require.Empty(t, actor.Domain)
	})

	t.Run("RegisterHasNoActor", func(t *testing.T) {
		err := ResolveRequestActor(c, inst, &permission.Permission{Type: permission.TypeRegister})
		require.NoError(t, err)

		_, ok := GetActor(c)
		require.False(t, ok)
	})
}
