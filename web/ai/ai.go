package ai

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/rag"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Chat is the route for asking a chat completion to AI.
func Chat(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.ChatConversations); err != nil {
		return middlewares.ErrForbidden
	}
	var payload rag.ChatPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}
	payload.ChatConversationID = c.Param("id")
	inst := middlewares.GetInstance(c)
	chat, err := rag.Chat(inst, payload)
	if err != nil {
		return jsonapi.InternalServerError(err)
	}
	return jsonapi.Data(c, http.StatusAccepted, chat, nil)
}

// Routes sets the routing for the AI tasks.
func Routes(router *echo.Group) {
	router.POST("/chat/conversations/:id", Chat)
}
