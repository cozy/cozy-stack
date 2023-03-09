package main

import (
	"net/http"
	"net/url"
)

func callback(w http.ResponseWriter, req *http.Request) {
	u := &url.URL{
		Scheme:   req.URL.Scheme,
		Host:     "localhost:8080",
		Path:     "/oidc/redirect",
		RawQuery: req.URL.RawQuery,
	}
	http.Redirect(w, req, u.String(), http.StatusSeeOther)
}

func main() {
	http.HandleFunc("/callback", callback)
	http.ListenAndServe(":4242", nil)
}
