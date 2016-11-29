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
	Passphrase string `form:"passphrase"`
	Token      string `form:"registerToken"`
}

func redirectSuccessLogin(c *gin.Context, slug string) {
	instance := middlewares.GetInstance(c)
	session, err := NewSession(instance)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(c.Writer, session.ToCookie())
	c.Redirect(http.StatusSeeOther, instance.SubDomain(slug))
}

func register(c *gin.Context) {
	instance := middlewares.GetInstance(c)

	var form registerForm
	if err := binding.Form.Bind(c.Request, &form); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	pass := []byte(form.Passphrase)
	token := []byte(form.Token)

	if err := instance.RegisterPassphrase(pass, token); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	redirectSuccessLogin(c, apps.OnboardingSlug)
}

func loginForm(c *gin.Context) {
	if IsLoggedIn(c) {
		instance := middlewares.GetInstance(c)
		c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.HomeSlug))
		return
	}
	c.HTML(http.StatusOK, "login.html", gin.H{})
}

func login(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	if IsLoggedIn(c) {
		c.Redirect(http.StatusSeeOther, instance.SubDomain(apps.HomeSlug))
		return
	}
	pass := c.PostForm("passphrase")
	if err := instance.CheckPassphrase([]byte(pass)); err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"InvalidPassphrase": true,
		})
		return
	}

	redirectSuccessLogin(c, apps.HomeSlug)
}

func logout(c *gin.Context) {
	// TODO check that a valid CtxToken is given to protect against CSRF attacks
	instance := middlewares.GetInstance(c)
	session, err := GetSession(c)
	if err == nil {
		http.SetCookie(c.Writer, session.Delete(instance))
	}
	c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login"))
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c *gin.Context) bool {
	_, err := GetSession(c)
	return err == nil
}

// Routes sets the routing for the status service
func Routes(router *gin.Engine) {
	router.POST("/register", middlewares.NeedInstance(), register)

	auth := router.Group("/auth", middlewares.NeedInstance())
	auth.GET("/login", loginForm)
	auth.POST("/login", login)
	auth.DELETE("/login", logout)
}
