// Package auth provides register and login handlers
package auth

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type registerForm struct {
	Passphrase string `form:"passphrase"`
	Token      string `form:"registerToken"`
}

type loginForm struct {
	Passphrase string `form:"passphrase"`
}

func redirectSuccessLogin(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	session, err := NewSession(instance)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(c.Writer, session.ToCookie())
	c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.OnboardingSlug))
}

func register(c *gin.Context) {
	instance := middlewares.GetInstance(c)

	var form registerForm
	if err := binding.Form.Bind(c.Request, &form); err != nil {
		fmt.Println(err, form)
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	pass := []byte(form.Passphrase)
	token := []byte(form.Token)

	if err := instance.RegisterPassphrase(pass, token); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	redirectSuccessLogin(c)
}

func login(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	var form loginForm
	if err := binding.Form.Bind(c.Request, &form); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	pass := []byte(form.Passphrase)

	if err := instance.CheckPassphrase(pass); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
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
