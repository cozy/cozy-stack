package swift

import (
	"net/http"
	"strconv"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/labstack/echo/v4"
)

func ListLayouts(c echo.Context) error {
	type layout struct {
		Counter int      `json:"counter"`
		Domains []string `json:"domains,omitempty"`
	}
	var layoutV1, layoutV2a, layoutV2b, layoutUnknown, layoutV3 layout

	flagShowDomains := false
	flagParam := c.QueryParam("show_domains")
	if converted, err := strconv.ParseBool(flagParam); err == nil {
		flagShowDomains = converted
	}

	instances, err := instance.List()
	if err != nil {
		return err
	}
	for _, inst := range instances {
		switch inst.SwiftLayout {
		case 0:
			layoutV1.Counter++
			if flagShowDomains {
				layoutV1.Domains = append(layoutV1.Domains, inst.Domain)
			}
		case 1:
			switch inst.DBPrefix() {
			case inst.Domain:
				layoutV2a.Counter++
				if flagShowDomains {
					layoutV2a.Domains = append(layoutV2a.Domains, inst.Domain)
				}
			case inst.Prefix:
				layoutV2b.Counter++
				if flagShowDomains {
					layoutV2b.Domains = append(layoutV2b.Domains, inst.Domain)
				}
			default:
				layoutUnknown.Counter++
				if flagShowDomains {
					layoutUnknown.Domains = append(layoutUnknown.Domains, inst.Domain)
				}
			}
		case 2:
			layoutV3.Counter++
			if flagShowDomains {
				layoutV3.Domains = append(layoutV3.Domains, inst.Domain)
			}
		default:
			layoutUnknown.Counter++
			if flagShowDomains {
				layoutUnknown.Domains = append(layoutUnknown.Domains, inst.Domain)
			}
		}
	}

	output := make(map[string]interface{})
	output["v1"] = layoutV1
	output["v2a"] = layoutV2a
	output["v2b"] = layoutV2b
	output["unknown"] = layoutUnknown
	output["v3"] = layoutV3
	output["total"] = layoutV1.Counter + layoutV2a.Counter + layoutV2b.Counter + layoutUnknown.Counter + layoutV3.Counter

	return c.JSON(http.StatusOK, output)
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.GET("/list-layouts", ListLayouts, checkSwift)
}

// checkSwift middleware ensures that the VFS relies on Swift
func checkSwift(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if config.FsURL().Scheme != config.SchemeSwift &&
			config.FsURL().Scheme != config.SchemeSwiftSecure {
			return c.JSON(http.StatusBadRequest, "the configured filesystem does not rely on OpenStack Swift")
		}
		return next(c)
	}
}
