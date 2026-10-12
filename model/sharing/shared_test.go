package sharing

import (
	"fmt"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveSharedRefsByPage(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()

	for i := 0; i < 5; i++ {
		infos := map[string]SharedInfo{"removed-sharing": {Rule: 0}}
		if i%2 == 0 {
			infos["other-sharing"] = SharedInfo{Rule: 0}
		}
		ref := &SharedRef{
			SID:       fmt.Sprintf("%s/file%d", consts.Files, i),
			Revisions: &RevsTree{Rev: "1-aaa"},
			Infos:     infos,
		}
		require.NoError(t, couchdb.CreateNamedDoc(inst, ref))
	}

	require.NoError(t, RemoveSharedRefsByPage(inst, "removed-sharing", 2))

	for i := 0; i < 5; i++ {
		var ref SharedRef
		err := couchdb.GetDoc(inst, consts.Shared, fmt.Sprintf("%s/file%d", consts.Files, i), &ref)
		if i%2 == 0 {
			require.NoError(t, err)
			assert.Equal(t, map[string]SharedInfo{"other-sharing": {Rule: 0}}, ref.Infos)
		} else {
			assert.True(t, couchdb.IsNotFoundError(err), "file%d: %v", i, err)
		}
	}
}
