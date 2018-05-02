package move

import "github.com/cozy/echo"

func exportsHandler(c echo.Context) error {
	return nil
}

func Routes(g *echo.Group) {
	g.GET("/exports", exportsHandler)
}
