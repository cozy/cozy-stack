package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
)

var accesscode = make(chan string, 1)

func checkError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

func userAcceptFunc(accessURL string, httpClient *http.Client) string {
	return <-accesscode
}

func handler(w http.ResponseWriter, r *http.Request) {
	for {
		val := r.URL.Query()
		if val.Get("access_code") != "" {
			accesscode <- val.Get("access_code")
			_, err := io.WriteString(w, "accesscode")
			checkError(err)
		}
	}
}

func main() {

	http.HandleFunc("/auth/accesscode", handler)
	go http.ListenAndServe(":8081", nil)

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	authClient := &auth.Client{
		RedirectURIs:    []string{"http://127.0.0.1:8081/auth/accesscode"},
		ClientName:      "Client",
		SoftwareID:      "github.com/cozy/cozy-stack/imexport",
		ClientKind:      "web",
		ClientURI:       "http://127.0.0.1:8081",
		SoftwareVersion: "0.0.1",
	}

	authReq := &auth.Request{
		ClientParams: authClient,
		HTTPClient:   httpClient,
		Domain:       "cozy.tools:8080",
	}

	// POST /auth/register
	authClient, err := authReq.RegisterClient(authClient)
	checkError(err)

	authReq.Scopes = append(authReq.Scopes, "io.cozy.files:GET")

	b := make([]byte, 32)
	_, err = io.ReadFull(rand.Reader, b)
	checkError(err)
	state := base64.StdEncoding.EncodeToString(b)

	// GET /auth/authorize
	codeURL, err := authReq.AuthCodeURL(authClient, state)
	checkError(err)

	fmt.Println(strings.Replace(codeURL, "https", "http", 1))

	code := userAcceptFunc(codeURL, httpClient)
	fmt.Println(code)

	// URL value
	v := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {authClient.ClientID},
		"client_secret": {authClient.ClientSecret},
	}

	// POST /auth/accessToken
	opts := &request.Options{
		Domain:  authReq.Domain,
		Scheme:  "http",
		Queries: v,
		Client:  httpClient,
		Method:  "POST",
		Path:    "/auth/access_token",
		Body:    strings.NewReader(v.Encode()),
		Headers: request.Headers{
			"Content-Type": "application/x-www-form-urlencoded",
			"Accept":       "application/json",
		},
	}

	accessToken := &auth.AccessToken{}

	resp, err := request.Req(opts)
	checkError(err)

	err = json.NewDecoder(resp.Body).Decode(&accessToken)
	checkError(err)

	bearerAuthz := &request.BearerAuthorizer{
		Token: accessToken.AccessToken,
	}

	opts.Authorizer = bearerAuthz

	file, err := os.Create("cozy.tar.gz")
	checkError(err)
	defer file.Close()

	err = Tardir(file, opts, authReq)
	checkError(err)

}
