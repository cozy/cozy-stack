package data

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

// TODO: make specific routes for managing accounts. The overriding of the
// /data/io.cozy.accounts/* routes is here mainly for retro-compatible reasons,
// but specific routes would improve the API.

func getAccount(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := consts.Accounts
	docid := c.Get("docid").(string)
	if docid == "" {
		return dbStatus(c)
	}

	var out couchdb.JSONDoc
	var err error
	rev := c.QueryParam("rev")
	if rev != "" {
		err = couchdb.GetDocRev(instance, doctype, docid, rev, &out)
	} else {
		err = couchdb.GetDoc(instance, doctype, docid, &out)
	}
	if err != nil {
		return fixErrorNoDatabaseIsWrongDoctype(err)
	}
	out.Type = doctype

	if err = middlewares.Allow(c, permission.GET, &out); err != nil {
		return err
	}

	if encryptAccount(out) {
		if err = couchdb.UpdateDoc(instance, out); err != nil {
			return err
		}
	}

	perm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if perm.Type == permission.TypeKonnector {
		decryptAccount(out)
	}

	return c.JSON(http.StatusOK, out.ToMapWithType())
}

func updateAccount(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	var doc couchdb.JSONDoc
	if err := json.NewDecoder(c.Request().Body).Decode(&doc); err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	doc.Type = consts.Accounts

	if (doc.ID() == "") != (doc.Rev() == "") {
		return jsonapi.NewError(http.StatusBadRequest,
			"You must either provide an _id and _rev in document (update) or neither (create with fixed id).")
	}

	if doc.ID() != "" && doc.ID() != c.Get("docid").(string) {
		return jsonapi.NewError(http.StatusBadRequest, "document _id doesnt match url")
	}

	if doc.ID() == "" {
		doc.SetID(c.Get("docid").(string))
		return createNamedDoc(c, doc)
	}

	errWhole := middlewares.AllowWholeType(c, permission.PUT, doc.DocType())
	if errWhole != nil {
		// we cant apply to whole type, let's fetch old doc and see if it applies there
		var old couchdb.JSONDoc
		errFetch := couchdb.GetDoc(instance, doc.DocType(), doc.ID(), &old)
		if errFetch != nil {
			return errFetch
		}
		old.Type = doc.DocType()
		// check if permissions set allows manipulating old doc
		errOld := middlewares.Allow(c, permission.PUT, &old)
		if errOld != nil {
			return errOld
		}

		// also check if permissions set allows manipulating new doc
		errNew := middlewares.Allow(c, permission.PUT, &doc)
		if errNew != nil {
			return errNew
		}
	}

	encryptAccount(doc)

	errUpdate := couchdb.UpdateDoc(instance, doc)
	if errUpdate != nil {
		return fixErrorNoDatabaseIsWrongDoctype(errUpdate)
	}

	perm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if perm.Type == permission.TypeKonnector {
		decryptAccount(doc)
	}

	return c.JSON(http.StatusOK, echo.Map{
		"ok":   true,
		"id":   doc.ID(),
		"rev":  doc.Rev(),
		"type": doc.DocType(),
		"data": doc.ToMapWithType(),
	})
}

func encryptAccount(doc couchdb.JSONDoc) bool {
	if config.GetVault().CredentialsEncryptorKey() != nil {
		return encryptMap(doc.M)
	}
	return false
}

func decryptAccount(doc couchdb.JSONDoc) bool {
	if config.GetVault().CredentialsDecryptorKey() != nil {
		return decryptMap(doc.M)
	}
	return false
}

func encryptMap(m map[string]interface{}) (encrypted bool) {
	auth, ok := m["auth"].(map[string]interface{})
	if !ok {
		return
	}
	login, _ := auth["login"].(string)
	cloned := make(map[string]interface{}, len(auth))
	var encKeys []string
	for k, v := range auth {
		var err error
		switch k {
		case "password":
			password, _ := v.(string)
			cloned["credentials_encrypted"], err = account.EncryptCredentials(login, password)
			if err == nil {
				encrypted = true
			}
		case "secret", "dob", "code", "answer", "access_token", "refresh_token", "appSecret", "session":
			cloned[k+"_encrypted"], err = account.EncryptCredentialsData(v)
			if err == nil {
				encrypted = true
			}
		default:
			if strings.HasSuffix(k, "_encrypted") {
				encKeys = append(encKeys, k)
			} else {
				cloned[k] = v
			}
		}
	}
	for _, key := range encKeys {
		if _, ok := cloned[key]; !ok {
			cloned[key] = auth[key]
		}
	}
	m["auth"] = cloned
	if data, ok := m["data"].(map[string]interface{}); ok {
		if encryptMap(data) && !encrypted {
			encrypted = true
		}
	}
	return
}

func decryptMap(m map[string]interface{}) (decrypted bool) {
	auth, ok := m["auth"].(map[string]interface{})
	if !ok {
		return
	}
	cloned := make(map[string]interface{}, len(auth))
	for k, v := range auth {
		if !strings.HasSuffix(k, "_encrypted") {
			cloned[k] = v
			continue
		}
		k = strings.TrimSuffix(k, "_encrypted")
		var str string
		str, ok = v.(string)
		if !ok {
			cloned[k] = v
			continue
		}
		var err error
		if k == "credentials" {
			cloned["login"], cloned["password"], err = account.DecryptCredentials(str)
		} else {
			cloned[k], err = account.DecryptCredentialsData(str)
		}
		if !decrypted {
			decrypted = err == nil
		}
	}
	m["auth"] = cloned
	if data, ok := m["data"].(map[string]interface{}); ok {
		if decryptMap(data) && !decrypted {
			decrypted = true
		}
	}
	return
}

func createAccount(c echo.Context) error {
	doctype := consts.Accounts
	instance := middlewares.GetInstance(c)

	doc := couchdb.JSONDoc{Type: doctype}
	if err := json.NewDecoder(c.Request().Body).Decode(&doc.M); err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	if err := middlewares.Allow(c, permission.POST, &doc); err != nil {
		return err
	}

	encryptAccount(doc)

	if err := couchdb.CreateDoc(instance, doc); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, echo.Map{
		"ok":   true,
		"id":   doc.ID(),
		"rev":  doc.Rev(),
		"type": doc.DocType(),
		"data": doc.ToMapWithType(),
	})
}
