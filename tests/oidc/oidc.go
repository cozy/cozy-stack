// This script can be used to start an OIDC for testing purpose. It can be
// useful for debugging the delegated authentication of the stack. You can run
// it with `go run ./tests/oidc`.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

type user struct {
	Name string `json:"name"`
	Sub  string `json:"sub"`
	Cozy string `json:"cozy,omitempty"`
}

var users = []user{
	{"Claude", "543d7eb8149c", "cozy.tools:8080"},
	{"Alice", "a83723b08d5f", "alice.cozy.tools:8080"},
	{"Bob", "bd14b6200614", "bob.cozy.tools:8080"},
}

// index is just a static page
func index(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%s index\n", r.Method)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<meta charset="utf-8">
<title>Fake OIDC server</title>
<h1>Fake OIDC server</h1>
<p>
This server is only for testing purpose.
You can put this in your config file to play with it:
<p>
<pre>
authentication:
  dev:
    oidc:
      client_id: my-client-id
      client_secret: my-client-secret
      scope: "openid profile"
      redirect_uri: http://cozy.tools:8080/oidc/redirect
      authorize_url: http://localhost:7007/authorize
      token_url: http://localhost:7007/token
      userinfo_url: http://localhost:7007/userinfo
      userinfo_instance_field: cozy
</pre>
</html>
`)
}

// authorize lets you authenticate as a user
func authorize(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("%s authorize\n", r.Method)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<meta charset="utf-8">
<title>Fake OIDC server</title>
<h1>Login as:</h1>
<ul>
`)
	state := r.URL.Query().Get("state")
	nonce := r.URL.Query().Get("nonce")
	redirectURI := r.URL.Query().Get("redirect_uri")
	redirect, err := url.Parse(redirectURI)
	if err != nil {
		w.WriteHeader(403)
		fmt.Printf("Cannot parse redirect_uri: %s\n", err)
		return
	}
	for _, u := range users {
		q := url.Values{}
		q.Add("code", "code-"+u.Sub)
		q.Add("state", state)
		q.Add("nonce", nonce)
		link := url.URL{
			Scheme:   redirect.Scheme,
			Host:     redirect.Host,
			Path:     redirect.Path,
			RawQuery: q.Encode(),
		}
		fmt.Fprintf(w, `<li><a href="%s">%s</a></li>`, link.String(), u.Name)
	}
	fmt.Fprintf(w, `</ul></html>`)
}

// token exchanges the temporary code to an access token
func token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	code := r.Form.Get("code")
	fmt.Printf("%s token (%s)\n", r.Method, code)
	var u *user
	for i := range users {
		if code == "code-"+users[i].Sub {
			u = &users[i]
		}
	}
	if u == nil {
		w.WriteHeader(403)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  "access-" + u.Sub,
		"refresh_token": "refresh-" + u.Sub,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"id_token":      "XXX",
	})
}

// userinfo returns a JSON with the information about the user
func userinfo(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	fmt.Printf("%s userinfo (%s)\n", r.Method, auth)
	var u *user
	for i := range users {
		if auth == "Bearer access-"+users[i].Sub {
			u = &users[i]
		}
	}
	if u == nil {
		w.WriteHeader(403)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(u)
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/authorize", authorize)
	http.HandleFunc("/token", token)
	http.HandleFunc("/userinfo", userinfo)

	fmt.Printf("Start server on http://localhost:7007/\n")
	log.Fatal(http.ListenAndServe(":7007", nil))
}
