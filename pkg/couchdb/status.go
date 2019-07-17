package couchdb

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/config/config"
)

// CheckStatus checks that the stack can talk to CouchDB, and returns an error
// if it is not the case.
func CheckStatus() error {
	u := config.CouchURL().String() + "/_up"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "application/json")
	auth := config.GetConfig().CouchDB.Auth
	if auth != nil {
		if p, ok := auth.Password(); ok {
			req.SetBasicAuth(auth.Username(), p)
		}
	}
	res, err := config.GetConfig().CouchDB.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Invalid responde code: %d", res.StatusCode)
	}
	return nil
}
