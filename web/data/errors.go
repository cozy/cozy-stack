package data

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/couchdb"
)

// HTTPStatus gives the http status for given error
func HTTPStatus(err error) (code int) {
	if os.IsNotExist(err) {
		code = http.StatusNotFound
	} else if os.IsExist(err) {
		code = http.StatusConflict
	} else if couchErr, isCouchErr := err.(*couchdb.Error); isCouchErr {
		code = couchErr.StatusCode
	}

	if code == 0 {
		code = http.StatusInternalServerError
	}

	return
}

func invalidDoctypeErr(doctype string) error {
	return fmt.Errorf("Invalid doctype '%s'", doctype)
}
