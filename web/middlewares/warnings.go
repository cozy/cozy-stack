package middlewares

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

// List returns a list of possible warnings associated with the instance.
func ListWarnings(i *instance.Instance) (warnings []*jsonapi.Error) {
	if err := i.MovedError(); err != nil {
		warnings = append(warnings, err)
	}
	notSigned, deadline := i.CheckTOSNotSignedAndDeadline()
	if notSigned && deadline >= instance.TOSWarning {
		tosLink, _ := i.ManagerURL(instance.ManagerTOSURL)
		warnings = append(warnings, &jsonapi.Error{
			Status: http.StatusPaymentRequired,
			Title:  "TOS Updated",
			Code:   "tos-updated",
			Detail: i.Translate("Terms of services have been updated"),
			Links:  &jsonapi.LinksList{Self: tosLink},
		})
	}
	return
}
