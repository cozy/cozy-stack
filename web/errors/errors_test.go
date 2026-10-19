package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func handle(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/files/", nil)
	rec := httptest.NewRecorder()
	ErrorHandler(err, echo.New().NewContext(req, rec))
	return rec
}

func TestErrorHandler(t *testing.T) {
	t.Run("LockTimeoutIsServiceUnavailable", func(t *testing.T) {
		for _, err := range []error{
			lock.ErrTooManyRetries,
			fmt.Errorf("create file: %w", lock.ErrTooManyRetries),
		} {
			rec := handle(t, err)

			assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
			assert.Equal(t, lockRetryAfter, rec.Header().Get(echo.HeaderRetryAfter))

			var body struct {
				Errors []struct {
					Status string `json:"status"`
					Detail string `json:"detail"`
				} `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Errors, 1)
			assert.Equal(t, "503", body.Errors[0].Status)
			assert.Equal(t, errInstanceBusy.Error(), body.Errors[0].Detail)
		}
	})

	t.Run("UnqualifiedErrorIsInternalServerError", func(t *testing.T) {
		rec := handle(t, fmt.Errorf("boom"))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Empty(t, rec.Header().Get(echo.HeaderRetryAfter))
	})
}
