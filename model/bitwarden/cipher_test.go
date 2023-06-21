package bitwarden

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCipher(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	_, _, err := stack.Start()
	require.NoError(t, err, "Error while starting the job system")

	t.Run("DeleteUnrecoverableCiphers", func(t *testing.T) {
		domain := "cozy.example.net"
		err := lifecycle.Destroy(domain)
		if !errors.Is(err, instance.ErrNotFound) {
			assert.NoError(t, err)
		}
		inst, err := lifecycle.Create(&lifecycle.Options{
			Domain:     domain,
			Passphrase: "cozy",
			PublicName: "Pierre",
		})
		assert.NoError(t, err)
		defer func() {
			_ = lifecycle.Destroy(inst.Domain)
		}()

		for i := 0; i < 5; i++ {
			md := metadata.New()
			md.DocTypeVersion = DocTypeVersion
			cipher := &Cipher{
				Type:           SecureNoteType,
				SharedWithCozy: i%2 == 0,
				Favorite:       i%3 == 0,
				Name:           fmt.Sprintf("2.%d|%d|%d", i, i, i),
				Metadata:       md,
			}
			assert.NoError(t, couchdb.CreateDoc(inst, cipher))
		}

		assert.NoError(t, DeleteUnrecoverableCiphers(inst))
		var ciphers []*Cipher
		err = couchdb.GetAllDocs(inst, consts.BitwardenCiphers, nil, &ciphers)
		assert.NoError(t, err)
		assert.Len(t, ciphers, 3)
		for _, c := range ciphers {
			assert.True(t, c.SharedWithCozy)
		}
	})
}
