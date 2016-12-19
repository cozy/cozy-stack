package settings

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/stretchr/testify/assert"
)

var TestPrefix = couchdb.SimpleDatabasePrefix("couchdb-tests")

func TestTheme(t *testing.T) {
	err := CreateDefaultTheme(TestPrefix)
	assert.NoError(t, err)
	theme, err := DefaultTheme(TestPrefix)
	assert.NoError(t, err)
	assert.Equal(t, "/assets/images/cozy-dev.svg", theme.Logo)
	assert.Equal(t, "#EAEEF2", theme.Base00)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	err := couchdb.ResetDB(TestPrefix, consts.Settings)
	if err != nil {
		fmt.Printf("Cant reset db (%s, %s) %s\n", TestPrefix, consts.Settings, err.Error())
		os.Exit(1)
	}
	res := m.Run()
	couchdb.DeleteDB(TestPrefix, consts.Settings)
	os.Exit(res)
}
