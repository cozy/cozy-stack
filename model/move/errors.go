package move

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

var (
	// ErrExportNotFound is used when a export document could not be found
	ErrExportNotFound = echo.NewHTTPError(http.StatusNotFound, "exports: not found")
	// ErrExportExpired is used when the export document has expired along with
	// its associated data.
	ErrExportExpired = echo.NewHTTPError(http.StatusNotFound, "exports: has expired")
	// ErrMACInvalid is used when the given MAC is not valid.
	ErrMACInvalid = echo.NewHTTPError(http.StatusUnauthorized, "exports: invalid mac")
	// ErrExportConflict is used when an export is already being perfomed.
	ErrExportConflict = echo.NewHTTPError(http.StatusConflict, "export: an archive is already being created")
	// ErrExportDoesNotContainIndex is used when we could not find the index data
	// in the archive.
	ErrExportDoesNotContainIndex = echo.NewHTTPError(http.StatusBadRequest, "export: archive does not contain index data")
	// ErrExportInvalidCursor is used when the given index cursor is invalid
	ErrExportInvalidCursor = echo.NewHTTPError(http.StatusBadRequest, "export: cursor is invalid")
)
