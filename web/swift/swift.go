package swift

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/ncw/swift"
)

func ListLayouts(c echo.Context) error {
	type layout struct {
		Counter int      `json:"counter"`
		Domains []string `json:"domains,omitempty"`
	}
	var layoutV1, layoutV2a, layoutV2b, layoutUnknown, layoutV3a, layoutV3b layout

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
			switch inst.DBPrefix() {
			case inst.Domain:
				layoutV3a.Counter++
				if flagShowDomains {
					layoutV3a.Domains = append(layoutV3a.Domains, inst.Domain)
				}
			case inst.Prefix:
				layoutV3b.Counter++
				if flagShowDomains {
					layoutV3b.Domains = append(layoutV3b.Domains, inst.Domain)
				}
			default:
				layoutUnknown.Counter++
				if flagShowDomains {
					layoutUnknown.Domains = append(layoutUnknown.Domains, inst.Domain)
				}
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
	output["v3a"] = layoutV3a
	output["v3b"] = layoutV3b
	output["total"] = layoutV1.Counter + layoutV2a.Counter + layoutV2b.Counter + layoutUnknown.Counter + layoutV3a.Counter + layoutV3b.Counter

	return c.JSON(http.StatusOK, output)
}

// GetObject retrieves a Swift object from an instance
func GetObject(c echo.Context) error {
	i := middlewares.GetInstance(c)
	object := c.Param("object")
	unescaped, err := url.PathUnescape(object)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	sc := config.GetSwiftConnection()
	_, err = sc.ObjectGet(swiftContainer(i), unescaped, buf, false, nil)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, buf.String())
}

// PutObject puts an object into Swift
func PutObject(c echo.Context) error {
	i := middlewares.GetInstance(c)

	contentType := c.Request().Header.Get("Content-Type")
	objectName := c.Param("object")
	unescaped, err := url.PathUnescape(objectName)
	if err != nil {
		return err
	}

	sc := config.GetSwiftConnection()
	f, err := sc.ObjectCreate(swiftContainer(i), unescaped, true, "", contentType, nil)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, c.Request().Body)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

// DeleteObject removes an object from Swift
func DeleteObject(c echo.Context) error {
	i := middlewares.GetInstance(c)
	objectName := c.Param("object")
	unescaped, err := url.PathUnescape(objectName)
	if err != nil {
		return err
	}

	sc := config.GetSwiftConnection()
	err = sc.ObjectDelete(swiftContainer(i), unescaped)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, nil)
}

// ListObjects list objects of an instance
func ListObjects(c echo.Context) error {
	i := middlewares.GetInstance(c)
	sc := config.GetSwiftConnection()
	container := swiftContainer(i)

	outNames := []string{}

	err := sc.ObjectsWalk(container, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		names, err := sc.ObjectNames(container, opts)
		if err == nil {
			outNames = append(outNames, names...)
		}
		return names, err
	})
	if err != nil {
		return err
	}

	out := struct {
		ObjectNameList []string `json:"objects_names"`
	}{
		outNames,
	}
	return c.JSON(http.StatusOK, out)
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	router.GET("/layouts", ListLayouts, checkSwift)
	router.GET("/vfs/:object", GetObject, checkSwift, middlewares.NeedInstance)
	router.PUT("/vfs/:object", PutObject, checkSwift, middlewares.NeedInstance)
	router.DELETE("/vfs/:object", DeleteObject, checkSwift, middlewares.NeedInstance)
	router.GET("/vfs", ListObjects, checkSwift, middlewares.NeedInstance)
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

// swiftContainer returns the container name for an instance
func swiftContainer(i *instance.Instance) string {
	switch i.SwiftLayout {
	case 0:
		return "cozy-" + i.DBPrefix()
	case 1:
		return "cozy-v2-" + i.DBPrefix()
	case 2:
		return "cozy-v3-" + i.DBPrefix()
	default:
		panic(errors.New("Unknown Swift layout"))
	}
}
