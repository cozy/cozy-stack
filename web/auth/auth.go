// Package auth provides register and login handlers
package auth

import (
	"net/http"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type registerForm struct {
	Password string `form:"password"`
	Token    string `form:"registerToken"`
}

type loginForm struct {
	Password string `form:"password"`
}

func redirectSuccessLogin(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	session, err := NewSession(instance)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
	}
	http.SetCookie(c.Writer, session.ToCookie())
	c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.OnboardingSlug))
}

func register(c *gin.Context) {
	instance := middlewares.GetInstance(c)

	var form registerForm
	if err := binding.Form.Bind(c.Request, &form); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
	}

	if err := instance.RegisterPassword(form.Password, form.Token); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
	}

	redirectSuccessLogin(c)
}

func login(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	var form loginForm
	if err := binding.Form.Bind(c.Request, &form); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
	}

	if err := instance.CheckPassword(form.Password); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
	}

	redirectSuccessLogin(c)
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c *gin.Context) bool {
	_, err := GetSession(c)
	return err == nil
}

// Routes sets the routing for the status service
func Routes(router gin.IRoutes) {
	router.POST("/register", register)
	router.POST("/login", login)
	// router.DELETE("/:doctype/:docid", DeleteDoc)
}
