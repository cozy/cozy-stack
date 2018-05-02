package move

import (
	"encoding/hex"
	"io"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/workers/move"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func exportsHandler(c echo.Context) error {
	if !middlewares.IsLoggedIn(c) {
		return echo.NewHTTPError(http.StatusUnauthorized)
	}
	inst := middlewares.GetInstance(c)
	exportID := c.Param("export-id")
	exportMAC, _ := hex.DecodeString(c.QueryParam("mac"))
	exportDoc, err := move.GetExport(inst, exportID, exportMAC)
	if err != nil {
		return err
	}

	archive, err := move.SystemArchiver().OpenArchive(exportDoc)
	if err != nil {
		return err
	}
	defer archive.Close()

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(c.Response(), archive, buf)
	return err
}

func Routes(g *echo.Group) {
	g.GET("/exports/:export-id", exportsHandler)
}
