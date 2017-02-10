package sharings

import (
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// ErrDocTypeInvalid is used when the document type sent is not
// recognized
var ErrDocTypeInvalid = errors.New("Invalid document type")

func checkStruct(c echo.Context) error {

	doctype := c.Get("type")
	if doctype != consts.OneShotSharing &&
		doctype != consts.MasterSlaveSharing &&
		doctype != consts.MasterMasterSharing {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Invalid document type",
		})
	}

	/*
		case consts.FileType:
			doc, err = createFileHandler(c, instance)
		case consts.DirType:
			doc, err = createDirHandler(c, instance)
		default:
			err = ErrDocTypeInvalid
		}
	*/
	return nil
}

// CreateSharing initializes a sharing by creating a document
func CreateSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	//check parameters
	//if err := checkStruct(c); err != nil {
	//	return c.JSON(err.Code, err)
	//}

	// create document

	sharing := new(sharings.Sharing)

	if err := c.Bind(sharing); err != nil {
		return err
	}

	doc, err := sharings.Create(instance, sharing)

	if err != nil {
		return err
	}

	// check each recipient and start oAuth dance

	return jsonapi.Data(c, http.StatusAccepted, doc, nil)

}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	// API Routes
	router.POST("/", CreateSharing)
}
