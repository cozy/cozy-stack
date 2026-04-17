package remote

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/model/nextcloud"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/webdav"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

const nextcloudMigrationLogNamespace = "nextcloud-migration"

type nextcloudMigrationRequest struct {
	NextcloudURL         string `json:"nextcloud_url"`
	NextcloudLogin       string `json:"nextcloud_login"`
	NextcloudAppPassword string `json:"nextcloud_app_password"`
	SourcePath           string `json:"source_path,omitempty"`
	TargetDir            string `json:"target_dir,omitempty"`
}

func (r *nextcloudMigrationRequest) normalize() error {
	r.NextcloudURL = strings.TrimSpace(r.NextcloudURL)
	r.NextcloudLogin = strings.TrimSpace(r.NextcloudLogin)
	r.NextcloudAppPassword = strings.TrimSpace(r.NextcloudAppPassword)
	r.SourcePath = strings.TrimSpace(r.SourcePath)
	r.TargetDir = strings.TrimSpace(r.TargetDir)

	required := []struct {
		name  string
		value string
	}{
		{"nextcloud_url", r.NextcloudURL},
		{"nextcloud_login", r.NextcloudLogin},
		{"nextcloud_app_password", r.NextcloudAppPassword},
	}
	for _, f := range required {
		if f.value == "" {
			return fmt.Errorf("%s is required", f.name)
		}
	}
	r.NextcloudURL = utils.EnsureHasSuffix(r.NextcloudURL, "/")
	if r.SourcePath == "" {
		r.SourcePath = "/"
	}
	if r.TargetDir != "" {
		r.TargetDir = strings.TrimRight(r.TargetDir, "/")
		if !strings.HasPrefix(r.TargetDir, "/") || path.Clean(r.TargetDir) != r.TargetDir {
			return errors.New("target_dir must be a clean absolute path")
		}
	}
	return nil
}

func (h *HTTPHandler) postNextcloudMigration(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.NextcloudMigrations); err != nil {
		return err
	}
	inst := middlewares.GetInstance(c)

	var body nextcloudMigrationRequest
	if err := c.Bind(&body); err != nil {
		return jsonapi.BadRequest(errors.New("invalid JSON body"))
	}
	if err := body.normalize(); err != nil {
		return jsonapi.BadRequest(err)
	}

	reqLogger := inst.Logger().WithNamespace(nextcloudMigrationLogNamespace).WithFields(logger.Fields{
		"nextcloud_host":  utils.ExtractInstanceHost(body.NextcloudURL),
		"nextcloud_login": body.NextcloudLogin,
	})

	doc, err := nextcloud.TriggerMigration(c.Request().Context(), inst, nextcloud.TriggerMigrationRequest{
		NextcloudURL:         body.NextcloudURL,
		NextcloudLogin:       body.NextcloudLogin,
		NextcloudAppPassword: body.NextcloudAppPassword,
		SourcePath:           body.SourcePath,
		TargetDir:            body.TargetDir,
	}, h.rmq, reqLogger)
	if err != nil {
		return mapNextcloudMigrationError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, doc, nil)
}

// mapNextcloudMigrationError translates the typed errors returned by
// [nextcloud.TriggerMigration] into the HTTP status codes the Settings UI
// expects. Unknown errors fall through as 500.
func mapNextcloudMigrationError(err error) error {
	switch {
	case errors.Is(err, nextcloud.ErrMigrationConflict):
		return jsonapi.Conflict(err)
	case errors.Is(err, webdav.ErrInvalidAuth):
		return jsonapi.Unauthorized(errors.New("nextcloud credentials are invalid"))
	case errors.Is(err, nextcloud.ErrNextcloudUnreachable):
		return jsonapi.BadGateway(fmt.Errorf("nextcloud unreachable: %w", err))
	case errors.Is(err, nextcloud.ErrMigrationBrokerUnavailable):
		return jsonapi.NewError(http.StatusServiceUnavailable, "migration service is unavailable, please retry later")
	default:
		return jsonapi.InternalServerError(err)
	}
}

// postNextcloudMigrationCancel publishes a cancel command for the given
// migration id. 202 means "cancel requested; poll the tracking document
// for the terminal state", not "migration stopped".
func (h *HTTPHandler) postNextcloudMigrationCancel(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.NextcloudMigrations); err != nil {
		return err
	}
	inst := middlewares.GetInstance(c)
	migrationID := c.Param("id")
	if migrationID == "" {
		return jsonapi.BadRequest(errors.New("missing migration id"))
	}

	reqLogger := inst.Logger().WithNamespace(nextcloudMigrationLogNamespace).WithFields(logger.Fields{
		"migration_id": migrationID,
	})
	ctx := logger.WithContext(c.Request().Context(), reqLogger)

	if err := nextcloud.CancelMigration(ctx, inst, migrationID, h.rmq); err != nil {
		return mapNextcloudMigrationCancelError(err)
	}
	return c.NoContent(http.StatusAccepted)
}

func mapNextcloudMigrationCancelError(err error) error {
	switch {
	case errors.Is(err, nextcloud.ErrMigrationNotFound):
		return jsonapi.NotFound(err)
	case errors.Is(err, nextcloud.ErrMigrationAlreadyTerminal):
		return jsonapi.Conflict(err)
	case errors.Is(err, nextcloud.ErrMigrationBrokerUnavailable):
		return jsonapi.NewError(http.StatusServiceUnavailable, "migration service is unavailable, please retry later")
	default:
		return jsonapi.InternalServerError(err)
	}
}
