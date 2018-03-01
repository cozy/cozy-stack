package data

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	perms "github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
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

	if err = permissions.Allow(c, permissions.GET, &out); err != nil {
		return err
	}

	if encryptAccount(out) {
		if err = couchdb.UpdateDoc(instance, out); err != nil {
			return err
		}
	}

	perm, err := permissions.GetPermission(c)
	if err != nil {
		return err
	}
	if perm.Type == perms.TypeKonnector {
		decryptAccount(out)
	}

	return c.JSON(http.StatusOK, out.ToMapWithType())
}

func updateAccount(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	var doc couchdb.JSONDoc
	if err := json.NewDecoder(c.Request().Body).Decode(&doc); err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
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

	errWhole := permissions.AllowWholeType(c, permissions.PUT, doc.DocType())
	if errWhole != nil {
		// we cant apply to whole type, let's fetch old doc and see if it applies there
		var old couchdb.JSONDoc
		errFetch := couchdb.GetDoc(instance, doc.DocType(), doc.ID(), &old)
		if errFetch != nil {
			return errFetch
		}
		old.Type = doc.DocType()
		// check if permissions set allows manipulating old doc
		errOld := permissions.Allow(c, permissions.PUT, &old)
		if errOld != nil {
			return errOld
		}

		// also check if permissions set allows manipulating new doc
		errNew := permissions.Allow(c, permissions.PUT, &doc)
		if errNew != nil {
			return errNew
		}
	}

	encryptAccount(doc)

	errUpdate := couchdb.UpdateDoc(instance, doc)
	if errUpdate != nil {
		return fixErrorNoDatabaseIsWrongDoctype(errUpdate)
	}

	perm, err := permissions.GetPermission(c)
	if err != nil {
		return err
	}
	if perm.Type == perms.TypeKonnector {
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
	return encryptMap(doc.M)
}

func decryptAccount(doc couchdb.JSONDoc) bool {
	return decryptMap(doc.M)
}

func encryptMap(m map[string]interface{}) bool {
	encrypted := false
	var passwordEncrypted []byte
	var err error
	for k, v := range m {
		var ok bool
		var password string
		if k == "password" {
			if password, ok = v.(string); ok && len(password) > 0 {
				login, _ := m["login"].(string)
				passwordEncrypted, err = accounts.EncryptCredentials(login, password)
				encrypted = err == nil
			}
		} else if mm, ok := v.(map[string]interface{}); ok && encryptMap(mm) {
			encrypted = true
		}
	}
	if len(passwordEncrypted) > 0 {
		delete(m, "password")
		m["credentials_encrypted"] = base64.StdEncoding.EncodeToString(passwordEncrypted)
	}
	return encrypted
}

func decryptMap(m map[string]interface{}) bool {
	decrypted := false
	var login, password string
	for k, v := range m {
		if k == "credentials_encrypted" {
			encodedEncryptedCreds, ok := v.(string)
			if ok {
				encryptedCreds, err := base64.StdEncoding.DecodeString(encodedEncryptedCreds)
				if err == nil {
					login, password, err = accounts.DecryptCredentials(encryptedCreds)
					decrypted = err == nil
				}
			}
		} else if mm, ok := v.(map[string]interface{}); ok && decryptMap(mm) {
			decrypted = true
		}
	}
	if decrypted {
		delete(m, "credentials_encrypted")
		m["login"] = login
		m["password"] = password
	}
	return decrypted
}

func createAccount(c echo.Context) error {
	doctype := consts.Accounts
	instance := middlewares.GetInstance(c)

	doc := couchdb.JSONDoc{Type: doctype}
	if err := json.NewDecoder(c.Request().Body).Decode(&doc.M); err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	if err := permissions.Allow(c, permissions.POST, &doc); err != nil {
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
