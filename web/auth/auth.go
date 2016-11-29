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
	c.HTML(http.StatusOK, "login.html", nil)
}

func login(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	pass := c.PostForm("passphrase")
	if err := instance.CheckPassphrase([]byte(pass)); err != nil {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{
			"InvalidPassphrase": true,
		})
		return
	}

	redirectSuccessLogin(c, apps.HomeSlug)
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c *gin.Context) bool {
	_, err := GetSession(c)
	return err == nil
}

// Routes sets the routing for the status service
func Routes(router gin.IRoutes) {
	router.POST("/register", middlewares.NeedInstance(), register)
	router.GET("/login", middlewares.NeedInstance(), loginForm)
	router.POST("/login", middlewares.NeedInstance(), login)
	// router.DELETE("/:doctype/:docid", DeleteDoc)
}
