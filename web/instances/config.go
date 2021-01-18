package instances

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/labstack/echo/v4"
)

func rebuildRedis(c echo.Context) error {
	instances, err := instance.List()
	if err != nil {
		return wrapError(err)
	}
	if err = job.System().CleanRedis(); err != nil {
		return wrapError(err)
	}
	for _, i := range instances {
		err = job.System().RebuildRedis(i)
		if err != nil {
			return wrapError(err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// Renders the assets list loaded in memory and served by the cozy
func assetsInfos(c echo.Context) error {
	assetsMap, err := assets.List()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, assetsMap)
}

func addAssets(c echo.Context) error {
	var unmarshaledAssets []model.AssetOption
	if err := json.NewDecoder(c.Request().Body).Decode(&unmarshaledAssets); err != nil {
		return err
	}

	err := assets.Add(unmarshaledAssets)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
	}
	return nil
}

func deleteAssets(c echo.Context) error {
	context := c.Param("context")
	name := c.Param("*")

	err := assets.Remove(name, context)
	if err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}
