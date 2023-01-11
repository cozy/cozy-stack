package data

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// XXX: it would be better to have specific routes for managing accounts. The
// overriding of the /data/io.cozy.accounts/* routes is here mainly for
// retro-compatible reasons, but specific routes would improve the API.

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
		err = couchdb.GetDoc(instance, consts.SoftDeletedAccounts, docid, &out)
		if err == nil && out.M["soft_deleted_rev"] != rev {
			err = errors.New("invalid rev")
		}
		if err != nil {
			err = couchdb.GetDocRev(instance, doctype, docid, rev, &out)
		}
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

	if account.Encrypt(out) {
		if err = couchdb.UpdateDoc(instance, &out); err != nil {
			return err
		}
	}

	perm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if perm.Type == permission.TypeKonnector ||
		(c.QueryParam("include") == "credentials" && perm.Type == permission.TypeWebapp) {
		// The account decryption is allowed for konnectors or for apps services
		account.Decrypt(out)
	}

	return c.JSON(http.StatusOK, out.ToMapWithType())
}

func updateAccount(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	docid := c.Get("docid").(string)

	var doc couchdb.JSONDoc
	if err := json.NewDecoder(c.Request().Body).Decode(&doc); err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	doc.Type = consts.Accounts

	if (doc.ID() == "") != (doc.Rev() == "") {
		return jsonapi.NewError(http.StatusBadRequest,
			"You must either provide an _id and _rev in document (update) or neither (create with fixed id).")
	}

	if doc.ID() != "" && doc.ID() != docid {
		return jsonapi.NewError(http.StatusBadRequest, "document _id doesnt match url")
	}

	if doc.ID() == "" {
		doc.SetID(docid)
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

	account.Encrypt(doc)

	errUpdate := couchdb.UpdateDoc(instance, &doc)
	if errUpdate != nil {
		return fixErrorNoDatabaseIsWrongDoctype(errUpdate)
	}

	perm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if perm.Type == permission.TypeKonnector {
		account.Decrypt(doc)
	}

	return c.JSON(http.StatusOK, echo.Map{
		"ok":   true,
		"id":   doc.ID(),
		"rev":  doc.Rev(),
		"type": doc.DocType(),
		"data": doc.ToMapWithType(),
	})
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

	account.Encrypt(doc)
	account.ComputeName(doc)

	if err := couchdb.CreateDoc(instance, &doc); err != nil {
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
