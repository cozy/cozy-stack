package binding

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func useMemStoreForTest(t *testing.T) {
	t.Helper()
	globalStore = &memStore{bindings: make(map[string]map[string]struct{})}
	storeOnce = sync.Once{}
	storeOnce.Do(func() {})
	t.Cleanup(func() {
		globalStore = nil
		storeOnce = sync.Once{}
	})
}

func TestFindProviderKeysAndTypedListings(t *testing.T) {
	useMemStoreForTest(t)

	require.NoError(t, BindSession("ctx-b", "b.example", "sid-1", "session-b"))
	require.NoError(t, BindSession("ctx-a", "a.example", "sid-1", "session-a"))
	require.NoError(t, BindOAuthClient("ctx-a", "a.example", "sid-1", "client-a"))
	require.NoError(t, BindOAuthClient("ctx-b", "b.example", "sid-1", "client-b"))

	keys, err := FindProviderKeys("sid-1")
	require.NoError(t, err)
	require.Equal(t, []string{"ctx-a", "ctx-b"}, keys)

	sessionRefs, err := ListSessions("", "sid-1")
	require.NoError(t, err)
	require.Len(t, sessionRefs, 2)
	require.Equal(t, sessionRef{
		OIDCProviderKey: "ctx-a",
		Domain:          "a.example",
		SessionID:       "session-a",
	}, sessionRefs[0])
	require.Equal(t, sessionRef{
		OIDCProviderKey: "ctx-b",
		Domain:          "b.example",
		SessionID:       "session-b",
	}, sessionRefs[1])

	filteredSessions, err := ListSessions("ctx-a", "sid-1")
	require.NoError(t, err)
	require.Equal(t, []sessionRef{{
		OIDCProviderKey: "ctx-a",
		Domain:          "a.example",
		SessionID:       "session-a",
	}}, filteredSessions)

	clientRefs, err := ListOAuthClients("", "sid-1")
	require.NoError(t, err)
	require.Len(t, clientRefs, 2)
	require.Equal(t, oauthClientRef{
		OIDCProviderKey: "ctx-a",
		Domain:          "a.example",
		OAuthClientID:   "client-a",
	}, clientRefs[0])
	require.Equal(t, oauthClientRef{
		OIDCProviderKey: "ctx-b",
		Domain:          "b.example",
		OAuthClientID:   "client-b",
	}, clientRefs[1])

	filteredClients, err := ListOAuthClients("ctx-b", "sid-1")
	require.NoError(t, err)
	require.Equal(t, []oauthClientRef{{
		OIDCProviderKey: "ctx-b",
		Domain:          "b.example",
		OAuthClientID:   "client-b",
	}}, filteredClients)
}

func TestUnbindRemovesTypedRefs(t *testing.T) {
	useMemStoreForTest(t)

	require.NoError(t, BindSession("ctx-a", "a.example", "sid-2", "session-a"))
	require.NoError(t, BindOAuthClient("ctx-a", "a.example", "sid-2", "client-a"))

	require.NoError(t, UnbindSession("ctx-a", "a.example", "sid-2", "session-a"))
	require.NoError(t, UnbindOAuthClient("ctx-a", "a.example", "sid-2", "client-a"))

	sessionRefs, err := ListSessions("", "sid-2")
	require.NoError(t, err)
	require.Empty(t, sessionRefs)

	clientRefs, err := ListOAuthClients("", "sid-2")
	require.NoError(t, err)
	require.Empty(t, clientRefs)

	keys, err := FindProviderKeys("sid-2")
	require.NoError(t, err)
	require.Empty(t, keys)
}
