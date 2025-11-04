package sharing

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/require"
)

func TestIsTrustedMember(t *testing.T) {
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()

	cfg := config.GetConfig()
	prevSharing := cfg.Sharing
	cfg.Sharing = config.SharingConfig{
		AutoAcceptTrusted: true,
		Contexts:          map[string]config.SharingContext{},
	}
	t.Cleanup(func() { cfg.Sharing = prevSharing })

	inst.Domain = "owner.example.com"

	t.Run("no instance URL", func(t *testing.T) {
		member := &Member{Email: "outsider@unknown.net"}
		require.False(t, IsTrustedMember(inst, member))
	})

	t.Run("untrusted instance domain", func(t *testing.T) {
		member := &Member{Email: "outsider@unknown.net", Instance: "https://stranger.unknown.net"}
		require.False(t, IsTrustedMember(inst, member))
	})

	t.Run("Scenario 1: B2C SaaS (twake.app) - users DON'T trust each other", func(t *testing.T) {
		// In B2C SaaS, users like alice.twake.app and bob.twake.app should NOT trust each other
		// Configuration: Don't add twake.app to TrustedDomains
		cfg.Sharing.Contexts[config.DefaultInstanceContext] = config.SharingContext{
			TrustedDomains: []string{}, // Empty - no trust between users
		}
		t.Cleanup(func() { delete(cfg.Sharing.Contexts, config.DefaultInstanceContext) })

		inst.Domain = "alice.twake.app"
		member := &Member{Email: "bob@twake.app", Instance: "https://bob.twake.app"}

		// Alice should NOT trust Bob (no trusted domains configured)
		require.False(t, IsTrustedMember(inst, member))
	})

	t.Run("Scenario 2: B2B SaaS org - users within org trust each other", func(t *testing.T) {
		// In B2B SaaS, users within the same organization should trust each other
		// Organization: linagora.twake.app
		// Users: alice.linagora.twake.app, bob.linagora.twake.app
		cfg.Sharing.Contexts[config.DefaultInstanceContext] = config.SharingContext{
			TrustedDomains: []string{"linagora.twake.app"},
		}
		t.Cleanup(func() { delete(cfg.Sharing.Contexts, config.DefaultInstanceContext) })

		t.Run("users in same org trust each other", func(t *testing.T) {
			inst.Domain = "alice.linagora.twake.app"
			member := &Member{Email: "bob@linagora.com", Instance: "https://bob.linagora.twake.app"}

			// Bob's domain ends with "linagora.twake.app" → should trust
			require.True(t, IsTrustedMember(inst, member))
		})

		t.Run("users from different orgs DON'T trust each other", func(t *testing.T) {
			inst.Domain = "alice.linagora.twake.app"
			member := &Member{Email: "charlie@acme.com", Instance: "https://charlie.acme.twake.app"}

			// Charlie's domain "charlie.acme.twake.app" doesn't end with "linagora.twake.app" → should NOT trust
			require.False(t, IsTrustedMember(inst, member))
		})

		t.Run("deeper subdomain levels within same org", func(t *testing.T) {
			inst.Domain = "alice.eng.linagora.twake.app"
			member := &Member{Email: "bob@linagora.com", Instance: "https://bob.sales.linagora.twake.app"}

			// Bob's domain ends with "linagora.twake.app" → should trust
			require.True(t, IsTrustedMember(inst, member))
		})

	})

	t.Run("Scenario 3: On-premise - all users trust each other", func(t *testing.T) {
		// On-premise deployment where all users under *.linagora.com trust each other
		cfg.Sharing.Contexts[config.DefaultInstanceContext] = config.SharingContext{
			TrustedDomains: []string{"linagora.com"},
		}
		t.Cleanup(func() { delete(cfg.Sharing.Contexts, config.DefaultInstanceContext) })

		t.Run("all users under same domain trust each other", func(t *testing.T) {
			inst.Domain = "alice.linagora.com"
			member := &Member{Email: "bob@linagora.com", Instance: "https://bob.linagora.com"}

			// Bob's domain ends with "linagora.com" → should trust
			require.True(t, IsTrustedMember(inst, member))
		})

		t.Run("subdomain levels trust each other", func(t *testing.T) {
			inst.Domain = "alice.eng.linagora.com"
			member := &Member{Email: "bob@linagora.com", Instance: "https://bob.sales.linagora.com"}

			// Bob's domain ends with "linagora.com" → should trust
			require.True(t, IsTrustedMember(inst, member))
		})

		t.Run("users from different root domains DON'T trust", func(t *testing.T) {
			inst.Domain = "alice.linagora.com"
			member := &Member{Email: "eve@evil.com", Instance: "https://eve.evil.com"}

			// Eve's domain doesn't end with "linagora.com" → should NOT trust
			require.False(t, IsTrustedMember(inst, member))
		})

	})
}
