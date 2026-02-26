package sharings_test

import (
	"encoding/json"
	"net/http"
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
