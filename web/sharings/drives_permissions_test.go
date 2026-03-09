package sharings_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"
)

const jsonAPIContentType = "application/vnd.api+json"

type sharedDrivePermissionFixture struct {
	env           *sharedDrivesEnv
	eOwner        *httpexpect.Expect
	ownerAppToken string
	sharingID     string
	productID     string
	fileID        string
}

func setupSharedDrivePermissionFixture(t *testing.T) *sharedDrivePermissionFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	env := setupSharedDrivesEnv(t)
	eOwner, _, _ := env.createClients(t)

	fileID := createFile(t, eOwner, env.firstRootDirID, "document.txt", env.acmeToken)
	return &sharedDrivePermissionFixture{
		env:           env,
		eOwner:        eOwner,
		ownerAppToken: env.acmeToken,
		sharingID:     env.firstSharingID,
		productID:     env.firstRootDirID,
		fileID:        fileID,
	}
}

func (f *sharedDrivePermissionFixture) newBettyClient(t *testing.T) (*httpexpect.Expect, string) {
	t.Helper()
	_, eBetty, _ := f.env.createClients(t)
	return eBetty, f.env.bettyToken
}

func (f *sharedDrivePermissionFixture) newDaveClient(t *testing.T) (*httpexpect.Expect, string) {
	t.Helper()
	_, _, eDave := f.env.createClients(t)
	return eDave, f.env.daveToken
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func makeSharedDrivePermissionPayload(
	t *testing.T,
	ruleType string,
	values []string,
	selector string,
	extraAttrs map[string]interface{},
) []byte {
	t.Helper()

	rule := map[string]interface{}{
		"type":   ruleType,
		"verbs":  []string{"GET"},
		"values": values,
	}
	if selector != "" {
		rule["selector"] = selector
	}

	attrs := map[string]interface{}{
		"permissions": map[string]interface{}{
			"files": rule,
		},
	}
	for k, v := range extraAttrs {
		attrs[k] = v
	}

	return mustJSON(t, map[string]interface{}{
		"data": map[string]interface{}{
			"type":       consts.Permissions,
			"attributes": attrs,
		},
	})
}

func makeSharedDrivePermissionPayloadWithVerbs(
	t *testing.T,
	ruleType string,
	values []string,
	verbs []string,
	extraAttrs map[string]interface{},
) []byte {
	t.Helper()

	rule := map[string]interface{}{
		"type":   ruleType,
		"verbs":  verbs,
		"values": values,
	}

	attrs := map[string]interface{}{
		"permissions": map[string]interface{}{
			"files": rule,
		},
	}
	for k, v := range extraAttrs {
		attrs[k] = v
	}

	return mustJSON(t, map[string]interface{}{
		"data": map[string]interface{}{
			"type":       consts.Permissions,
			"attributes": attrs,
		},
	})
}

func makeSharedDrivePermissionPayloadWithPermissions(
	t *testing.T,
	perms map[string]interface{},
	extraAttrs map[string]interface{},
) []byte {
	t.Helper()

	attrs := map[string]interface{}{
		"permissions": perms,
	}
	for k, v := range extraAttrs {
		attrs[k] = v
	}

	return mustJSON(t, map[string]interface{}{
		"data": map[string]interface{}{
			"type":       consts.Permissions,
			"attributes": attrs,
		},
	})
}

func makeSharedDrivePatchPayload(t *testing.T, attrs map[string]interface{}) []byte {
	t.Helper()
	return mustJSON(t, map[string]interface{}{
		"data": map[string]interface{}{
			"type":       consts.Permissions,
			"attributes": attrs,
		},
	})
}

func createSharedDrivePermission(
	t *testing.T,
	client *httpexpect.Expect,
	sharingID, token, code, ttl string,
	payload []byte,
) (string, *httpexpect.Object) {
	t.Helper()

	req := client.POST("/sharings/drives/" + sharingID + "/permissions")
	if code != "" {
		req = req.WithQuery("codes", code)
	}
	if ttl != "" {
		req = req.WithQuery("ttl", ttl)
	}
	if token != "" {
		req = req.WithHeader("Authorization", "Bearer "+token)
	}

	obj := req.
		WithHeader("Content-Type", jsonAPIContentType).
		WithBytes(payload).
		Expect().Status(http.StatusOK).
		JSON(httpexpect.ContentOpts{MediaType: jsonAPIContentType}).
		Object()

	data := obj.Value("data").Object()
	permID := data.Value("id").String().NotEmpty().Raw()
	attrs := data.Value("attributes").Object()
	return permID, attrs
}

func createSharedDrivePermissionExpectStatus(
	t *testing.T,
	client *httpexpect.Expect,
	sharingID, token, code, ttl string,
	payload []byte,
	status int,
) {
	t.Helper()

	req := client.POST("/sharings/drives/" + sharingID + "/permissions")
	if code != "" {
		req = req.WithQuery("codes", code)
	}
	if ttl != "" {
		req = req.WithQuery("ttl", ttl)
	}
	if token != "" {
		req = req.WithHeader("Authorization", "Bearer "+token)
	}

	req.WithHeader("Content-Type", jsonAPIContentType).
		WithBytes(payload).
		Expect().Status(status)
}

func patchSharedDrivePermissionExpectStatus(
	t *testing.T,
	client *httpexpect.Expect,
	sharingID, permID, token string,
	payload []byte,
	status int,
) *httpexpect.Object {
	t.Helper()

	req := client.PATCH("/sharings/drives/" + sharingID + "/permissions/" + permID)
	if token != "" {
		req = req.WithHeader("Authorization", "Bearer "+token)
	}

	resp := req.WithHeader("Content-Type", jsonAPIContentType).
		WithBytes(payload).
		Expect().Status(status)

	if status != http.StatusOK {
		return nil
	}
	return resp.JSON(httpexpect.ContentOpts{MediaType: jsonAPIContentType}).Object()
}

func deleteSharedDrivePermissionExpectStatus(
	t *testing.T,
	client *httpexpect.Expect,
	sharingID, permID, token string,
	status int,
) {
	t.Helper()

	req := client.DELETE("/sharings/drives/" + sharingID + "/permissions/" + permID)
	if token != "" {
		req = req.WithHeader("Authorization", "Bearer "+token)
	}
	req.Expect().Status(status)
}

func listSharedDrivePermissionsExpectStatus(
	t *testing.T,
	client *httpexpect.Expect,
	sharingID string,
	ids []string,
	token string,
	status int,
) *httpexpect.Object {
	t.Helper()

	req := client.GET("/sharings/drives/" + sharingID + "/permissions")
	if len(ids) > 0 {
		req = req.WithQuery("ids", strings.Join(ids, ","))
	}
	if token != "" {
		req = req.WithHeader("Authorization", "Bearer "+token)
	}

	resp := req.Expect().Status(status)
	if status != http.StatusOK {
		return nil
	}
	return resp.JSON(httpexpect.ContentOpts{MediaType: jsonAPIContentType}).Object()
}

func createReadOnlyDriveForDave(
	t *testing.T,
	f *sharedDrivePermissionFixture,
) (string, string) {
	t.Helper()

	rootDirID := createRootDirectory(t, f.eOwner, testify(t, "dave-drive"), f.ownerAppToken)
	daveContact := createContact(t, f.env.acme, "Dave", "dave@example.net")

	obj := f.eOwner.POST("/sharings/drives").
		WithHeader("Authorization", "Bearer "+f.ownerAppToken).
		WithHeader("Content-Type", jsonAPIContentType).
		WithBytes(mustJSON(t, map[string]interface{}{
			"data": map[string]interface{}{
				"type": consts.Sharings,
				"attributes": map[string]interface{}{
					"description": "Drive for Dave list permissions",
					"folder_id":   rootDirID,
				},
				"relationships": map[string]interface{}{
					"read_only_recipients": map[string]interface{}{
						"data": []map[string]interface{}{
							{
								"id":   daveContact.ID(),
								"type": consts.Contacts,
							},
						},
					},
				},
			},
		})).
		Expect().Status(http.StatusCreated).
		JSON(httpexpect.ContentOpts{MediaType: jsonAPIContentType}).
		Object()

	sharingID := obj.Value("data").Object().Value("id").String().NotEmpty().Raw()
	acceptSharedDrive(t, f.env.acme, f.env.dave, "Dave", f.env.tsA.URL, f.env.tsD.URL, sharingID)
	return sharingID, rootDirID
}

func TestSharedDriveShareByLinkCreate(t *testing.T) {
	f := setupSharedDrivePermissionFixture(t)

	t.Run("OwnerCanCreateShareByLink", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, attrs := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)

		shortcodes := attrs.Value("shortcodes").Object()
		shortcodes.Value("link").String().NotEmpty()

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("RecipientCanCreateShareByLink", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, attrs := createSharedDrivePermission(
			t, eBetty, f.sharingID, bettyToken, "link", "", payload,
		)

		attrs.Value("shortcodes").Object().Value("link").String().NotEmpty()
		attrs.Value("cozyMetadata").Object().Value("createdByApp").String().IsEqual("drive")
		shareCode := attrs.Value("codes").Object().Value("link").String().NotEmpty().Raw()

		// Download the file using the share-by-link (on owner's instance where the file resides)
		f.eOwner.GET("/files/download/"+f.fileID).
			WithHeader("Authorization", "Bearer "+shareCode).
			Expect().Status(http.StatusOK)

		deleteSharedDrivePermissionExpectStatus(t, eBetty, f.sharingID, permID, bettyToken, http.StatusNoContent)
	})

	t.Run("FailOnFileOutsideSharedDrive", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(
			t, consts.Files, []string{f.env.outsideOfShareID}, "", nil,
		)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusBadRequest,
		)
	})

	t.Run("FailOnEmptyValues", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{}, "", nil)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusBadRequest,
		)
	})

	t.Run("FailOnMultipleValues", func(t *testing.T) {
		secondFileID := createFile(t, f.eOwner, f.productID, "document-2.txt", f.ownerAppToken)
		payload := makeSharedDrivePermissionPayload(
			t, consts.Files, []string{f.fileID, secondFileID}, "", nil,
		)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusBadRequest,
		)
	})

	t.Run("FailOnWildcardType", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, "*", []string{f.fileID}, "", nil)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusBadRequest,
		)
	})

	t.Run("FailOnSelector", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(
			t, consts.Files, []string{f.fileID}, "referenced_by", nil,
		)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusBadRequest,
		)
	})

	t.Run("FailOnMultiplePermissionRules", func(t *testing.T) {
		secondFileID := createFile(t, f.eOwner, f.productID, "document-3.txt", f.ownerAppToken)
		payload := makeSharedDrivePermissionPayloadWithPermissions(t, map[string]interface{}{
			"first": map[string]interface{}{
				"type":   consts.Files,
				"verbs":  []string{"GET"},
				"values": []string{f.fileID},
			},
			"second": map[string]interface{}{
				"type":   consts.Files,
				"verbs":  []string{"GET"},
				"values": []string{secondFileID},
			},
		}, nil)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusBadRequest,
		)
	})

	t.Run("FailWhenCallerCannotReadTargetFile", func(t *testing.T) {
		tagsOnlyToken := generateAppToken(f.env.acme, "tags-app", "io.cozy.tags")
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, tagsOnlyToken, "link", "", payload, http.StatusForbidden,
		)
	})

	t.Run("FailOnUnauthorizedAccess", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, "", "link", "", payload, http.StatusUnauthorized,
		)
	})

	t.Run("CreateShareByLinkForDirectory", func(t *testing.T) {
		subDirID := createDirectory(t, f.eOwner, f.productID, "subdir", f.ownerAppToken)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{subDirID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "dir-link", "", payload,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("CreateShareByLinkWithPassword", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(
			t, consts.Files, []string{f.fileID}, "", map[string]interface{}{"password": "secret123"},
		)
		permID, attrs := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "pwd-link", "", payload,
		)

		attrs.Value("password").Boolean().IsTrue()
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("CreateShareByLinkWithTTL", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, attrs := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "ttl-link", "24h", payload,
		)

		attrs.Value("expires_at").String().NotEmpty()
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("FailWhenLinkAlreadyExistsForTarget", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)

		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusConflict,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("FailWhenAnotherMemberAlreadyCreatedLinkForTarget", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, eBetty, f.sharingID, bettyToken, "link", "", payload,
		)

		createSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload, http.StatusConflict,
		)

		deleteSharedDrivePermissionExpectStatus(t, eBetty, f.sharingID, permID, bettyToken, http.StatusNoContent)
	})
}

func TestSharedDriveShareByLinkPatch(t *testing.T) {
	f := setupSharedDrivePermissionFixture(t)

	t.Run("OwnerCanPatchPermission", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, eBetty, f.sharingID, bettyToken, "patch-test-link", "", payload,
		)

		patchedObj := patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": "new-password"}),
			http.StatusOK,
		)
		patchedObj.Value("data").Object().
			Value("attributes").Object().
			Value("password").Boolean().IsTrue()

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("CreatorCanPatchOwnPermission", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, eBetty, f.sharingID, bettyToken, "creator-patch-link", "", payload,
		)

		patchedObj := patchSharedDrivePermissionExpectStatus(
			t,
			eBetty,
			f.sharingID,
			permID,
			bettyToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"expires_at": "2030-01-01T00:00:00Z"}),
			http.StatusOK,
		)
		patchedObj.Value("data").Object().
			Value("attributes").Object().
			Value("expires_at").String().NotEmpty()

		patchSharedDrivePermissionExpectStatus(
			t,
			eBetty,
			f.sharingID,
			permID,
			bettyToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": "second-patch"}),
			http.StatusOK,
		)

		deleteSharedDrivePermissionExpectStatus(t, eBetty, f.sharingID, permID, bettyToken, http.StatusNoContent)
	})

	t.Run("PublicShareTokenCannotPatchPermission", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, attrs := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)
		shareCode := attrs.Value("codes").Object().Value("link").String().NotEmpty().Raw()

		patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			shareCode,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": "forbidden"}),
			http.StatusForbidden,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("PatchPasswordSetAndClear", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "patch-pwd-link", "", payload,
		)

		patchedObj := patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": "my-password"}),
			http.StatusOK,
		)
		patchedObj.Value("data").Object().
			Value("attributes").Object().
			Value("password").Boolean().IsTrue()

		clearedObj := patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": ""}),
			http.StatusOK,
		)
		clearedObj.Value("data").Object().
			Value("attributes").Object().
			NotContainsKey("password")

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("PatchExpiresAtSetAndClear", func(t *testing.T) {
		createPayload := makeSharedDrivePermissionPayload(
			t,
			consts.Files,
			[]string{f.fileID},
			"",
			map[string]interface{}{"expires_at": "2030-01-01T00:00:00Z"},
		)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "patch-ttl-link", "", createPayload,
		)

		patchedObj := patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"expires_at": "2031-01-01T00:00:00Z"}),
			http.StatusOK,
		)
		patchedObj.Value("data").Object().
			Value("attributes").Object().
			Value("expires_at").String().IsEqual("2031-01-01T00:00:00Z")

		clearedObj := patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"expires_at": ""}),
			http.StatusOK,
		)
		clearedObj.Value("data").Object().
			Value("attributes").Object().
			NotContainsKey("expires_at")

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("CannotPatchCodesOrPermissions", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "no-codes-patch-link", "", payload,
		)

		patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{
				"codes": map[string]interface{}{"new-code": "abc123"},
			}),
			http.StatusBadRequest,
		)

		patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{
				"permissions": map[string]interface{}{
					"files": map[string]interface{}{
						"type":   consts.Files,
						"verbs":  []string{"GET", "POST"},
						"values": []string{f.fileID},
					},
				},
			}),
			http.StatusBadRequest,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("PatchRejectsInvalidPasswordType", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)

		patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": false}),
			http.StatusBadRequest,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("PatchRejectsInvalidExpiresAtType", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)

		patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			permID,
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"expires_at": false}),
			http.StatusBadRequest,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("PatchNonExistentPermission", func(t *testing.T) {
		patchSharedDrivePermissionExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			"non-existent-id",
			f.ownerAppToken,
			makeSharedDrivePatchPayload(t, map[string]interface{}{"password": "test"}),
			http.StatusNotFound,
		)
	})
}

func TestSharedDriveShareByLinkList(t *testing.T) {
	f := setupSharedDrivePermissionFixture(t)

	t.Run("OwnerCanListDriveLinks", func(t *testing.T) {
		secondFileID := createFile(t, f.eOwner, f.productID, "list-owner.txt", f.ownerAppToken)
		readOnlyPermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"readonly-link",
			"",
			makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil),
		)
		writePermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"write-link",
			"",
			makeSharedDrivePermissionPayloadWithVerbs(
				t, consts.Files, []string{secondFileID}, []string{"GET", "POST"}, nil,
			),
		)

		obj := listSharedDrivePermissionsExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			[]string{f.fileID, secondFileID},
			f.ownerAppToken,
			http.StatusOK,
		)
		data := obj.Value("data").Array()
		data.Length().IsEqual(2)
		for _, item := range data.Iter() {
			item.Object().Value("attributes").Object().Value("codes").Object().NotEmpty()
		}

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, readOnlyPermID, f.ownerAppToken, http.StatusNoContent)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, writePermID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("WriteRecipientCanSeeWritableAndReadonlyLinks", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		secondFileID := createFile(t, f.eOwner, f.productID, "list-betty.txt", f.ownerAppToken)
		readOnlyPermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"readonly-link",
			"",
			makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil),
		)
		writePermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"write-link",
			"",
			makeSharedDrivePermissionPayloadWithVerbs(
				t, consts.Files, []string{secondFileID}, []string{"GET", "POST"}, nil,
			),
		)

		obj := listSharedDrivePermissionsExpectStatus(
			t,
			eBetty,
			f.sharingID,
			[]string{f.fileID, secondFileID},
			bettyToken,
			http.StatusOK,
		)
		obj.Value("data").Array().Length().IsEqual(2)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, readOnlyPermID, f.ownerAppToken, http.StatusNoContent)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, writePermID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("OwnerCanListOnlyRequestedDriveLinks", func(t *testing.T) {
		secondFileID := createFile(t, f.eOwner, f.productID, "list-page-2.txt", f.ownerAppToken)
		thirdFileID := createFile(t, f.eOwner, f.productID, "list-page-3.txt", f.ownerAppToken)
		fourthFileID := createFile(t, f.eOwner, f.productID, "list-page-4.txt", f.ownerAppToken)
		firstPermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"page-link-1",
			"",
			makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil),
		)
		secondPermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"page-link-2",
			"",
			makeSharedDrivePermissionPayload(t, consts.Files, []string{secondFileID}, "", nil),
		)
		thirdPermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			f.sharingID,
			f.ownerAppToken,
			"page-link-3",
			"",
			makeSharedDrivePermissionPayload(t, consts.Files, []string{thirdFileID}, "", nil),
		)
		obj := listSharedDrivePermissionsExpectStatus(
			t,
			f.eOwner,
			f.sharingID,
			[]string{f.fileID, thirdFileID, fourthFileID},
			f.ownerAppToken,
			http.StatusOK,
		)
		obj.Value("data").Array().Length().IsEqual(2)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, firstPermID, f.ownerAppToken, http.StatusNoContent)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, secondPermID, f.ownerAppToken, http.StatusNoContent)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, thirdPermID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("ListingRequiresIDs", func(t *testing.T) {
		f.eOwner.GET("/sharings/drives/"+f.sharingID+"/permissions").
			WithHeader("Authorization", "Bearer "+f.ownerAppToken).
			Expect().Status(http.StatusUnprocessableEntity)
	})

	t.Run("ReadOnlyRecipientSeesOnlyReadonlyLinks", func(t *testing.T) {
		eDave, daveToken := f.newDaveClient(t)
		sharingID, productID := createReadOnlyDriveForDave(t, f)

		readOnlyFileID := createFile(t, f.eOwner, productID, "list-dave.txt", f.ownerAppToken)
		secondFileID := createFile(t, f.eOwner, productID, "list-dave-write.txt", f.ownerAppToken)
		readOnlyPermID, readOnlyAttrs := createSharedDrivePermission(
			t,
			f.eOwner,
			sharingID,
			f.ownerAppToken,
			"readonly-link",
			"",
			makeSharedDrivePermissionPayload(t, consts.Files, []string{readOnlyFileID}, "", nil),
		)
		writePermID, _ := createSharedDrivePermission(
			t,
			f.eOwner,
			sharingID,
			f.ownerAppToken,
			"write-link",
			"",
			makeSharedDrivePermissionPayloadWithVerbs(
				t, consts.Files, []string{secondFileID}, []string{"GET", "POST"}, nil,
			),
		)
		readOnlyCode := readOnlyAttrs.Value("codes").Object().Value("readonly-link").String().Raw()

		obj := listSharedDrivePermissionsExpectStatus(
			t,
			eDave,
			sharingID,
			[]string{readOnlyFileID, secondFileID},
			daveToken,
			http.StatusOK,
		)
		data := obj.Value("data").Array()
		data.Length().IsEqual(1)
		attrs := data.Element(0).Object().Value("attributes").Object()
		attrs.Value("codes").Object().Value("readonly-link").String().IsEqual(readOnlyCode)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, sharingID, readOnlyPermID, f.ownerAppToken, http.StatusNoContent)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, sharingID, writePermID, f.ownerAppToken, http.StatusNoContent)
	})
}

func TestSharedDriveShareByLinkRevoke(t *testing.T) {
	f := setupSharedDrivePermissionFixture(t)

	t.Run("OwnerCanRevokePermissionCreatedByRecipient", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, eBetty, f.sharingID, bettyToken, "link", "", payload,
		)

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("CreatorCanRevokeOwnPermission", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, eBetty, f.sharingID, bettyToken, "link", "", payload,
		)

		deleteSharedDrivePermissionExpectStatus(t, eBetty, f.sharingID, permID, bettyToken, http.StatusNoContent)
	})

	t.Run("NonCreatorCannotRevokePermission", func(t *testing.T) {
		eBetty, bettyToken := f.newBettyClient(t)
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, _ := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)

		deleteSharedDrivePermissionExpectStatus(t, eBetty, f.sharingID, permID, bettyToken, http.StatusForbidden)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("PublicShareTokenCannotRevokePermission", func(t *testing.T) {
		payload := makeSharedDrivePermissionPayload(t, consts.Files, []string{f.fileID}, "", nil)
		permID, attrs := createSharedDrivePermission(
			t, f.eOwner, f.sharingID, f.ownerAppToken, "link", "", payload,
		)
		shareCode := attrs.Value("codes").Object().Value("link").String().NotEmpty().Raw()

		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, shareCode, http.StatusForbidden)
		deleteSharedDrivePermissionExpectStatus(t, f.eOwner, f.sharingID, permID, f.ownerAppToken, http.StatusNoContent)
	})

	t.Run("RevokeNonExistentPermission", func(t *testing.T) {
		deleteSharedDrivePermissionExpectStatus(
			t, f.eOwner, f.sharingID, "non-existent-id", f.ownerAppToken, http.StatusNotFound,
		)
	})
}
