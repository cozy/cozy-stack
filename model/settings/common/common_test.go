package common

import (
	"errors"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/require"
)

func TestUpdateCommonSettings_VersionMismatchReturnsTypedError(t *testing.T) {
	// Initialize test configuration
	config.UseTestFile(t)

	// Configure common settings to enable code path
	conf := config.GetConfig()
	conf.CommonSettings = map[string]config.CommonSettings{
		config.DefaultInstanceContext: {URL: "http://example.org", Token: "token"},
		"":                            {URL: "http://example.org", Token: "token"},
	}

	// Stub HTTP call to avoid network
	oldDo := DoCommonHTTP
	DoCommonHTTP = func(method, urlStr, token string, body []byte) error { return nil }
	defer func() { DoCommonHTTP = oldDo }()

	// Stub remote getter to simulate mismatch
	oldGet := GetRemoteCommonSettings
	GetRemoteCommonSettings = func(inst *instance.Instance) (*UserSettingsRequest, error) {
		return &UserSettingsRequest{Version: inst.CommonSettingsVersion + 1}, nil
	}
	defer func() { GetRemoteCommonSettings = oldGet }()

	inst := &instance.Instance{Domain: "foo.example", ContextName: config.DefaultInstanceContext}
	inst.CommonSettingsVersion = 1

	// Minimal settings doc with a common field to trigger path
	settings := &couchdb.JSONDoc{M: map[string]interface{}{"email": "a@b.c"}}

	updated, err := UpdateCommonSettings(inst, settings)
	require.Error(t, err)
	require.False(t, updated)
	require.True(t, errors.Is(err, ErrCommonSettingsVersionMismatch))
	require.Equal(t, 1, inst.CommonSettingsVersion, "version must not change on mismatch")
}
