package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
)

var access_code = "access_code"

func CheckError(err error) {
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

func UserAcceptFunc(accessURL string) string {
	for access_code == "access_code" {
	}
	code := access_code
	access_code = "access_code"
	return code
}

func handler(w http.ResponseWriter, r *http.Request) {
	val := r.URL.Query()
	access_code = val.Get("access_code")

	io.WriteString(w, "access_code")

}

func main() {

	http.HandleFunc("/oauth/access_code", handler)
	go http.ListenAndServe(":8081", nil)

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	authClient := &auth.Client{
		RedirectURIs:    []string{"http://127.0.0.1:8081/oauth/access_code"},
		ClientName:      "Client",
		SoftwareID:      "github.com/example/client",
		ClientKind:      "web",
		ClientURI:       "http://127.0.0.1:8081",
		SoftwareVersion: "2.0.1",
	}

	authReq := &auth.Request{
		ClientParams: authClient,
		HTTPClient:   httpClient,
		Domain:       "cozy.tools:8080",
	}

	// POST /auth/register
	authClient, err := authReq.RegisterClient(authClient)
	CheckError(err)

	authReq.Scopes = append(authReq.Scopes, "io.cozy.files:GET")

	b := make([]byte, 32)
	_, err = io.ReadFull(rand.Reader, b)
	CheckError(err)
	state := base64.StdEncoding.EncodeToString(b)

	// GET /auth/authorize
	codeURL, err := authReq.AuthCodeURL(authClient, state)
	CheckError(err)

	fmt.Println(strings.Replace(codeURL, "https", "http", 1))

	code := UserAcceptFunc(codeURL)

	// URL value
	v := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {authClient.ClientID},
		"client_secret": {authClient.ClientSecret},
	}

	// POST /auth/access_token
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

	access_token := &auth.AccessToken{}

	resp, err := httpClient.Get("http://127.0.0.1:8081/oauth/access_code")
	CheckError(err)

	var data []byte

	data, err = ioutil.ReadAll(resp.Body)
	CheckError(err)
	fmt.Println(string(data))

	resp, err = request.Req(opts)
	CheckError(err)

	data, err = ioutil.ReadAll(resp.Body)
	CheckError(err)
	err = json.Unmarshal(data, &access_token)
	CheckError(err)

}
