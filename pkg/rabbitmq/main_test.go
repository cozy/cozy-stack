package rabbitmq_test

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/spf13/viper"
)

// TestMain bootstraps the views and indexes that production stacks create
// at startup via stack.Start → couchdb.InitGlobalDB. Tests in this package
// call lifecycle.Create / instance.Get directly without going through
// testutils.NewSetup → GetTestInstance → stack.Start, so they never trigger
// that init themselves.
//
// They've historically relied on the side effect that earlier model/* test
// packages bootstrap the global DB and that the design doc persists in the
// shared CouchDB service across test binaries — but Go's test result cache
// can let those packages be skipped (cached "ok"), and then this implicit
// dependency breaks. See the CI flake debugged on PR #4716: the second
// lifecycle.Create call inside TestSyncCreatedOrgContact failed with
// "CouchDB(not_found): missing" because instance.Service.Get queried
// _design/domain-and-aliases on the global instances DB and that design
// doc had never been created.
//
// TODO: the more robust fix is to make instance.Service.Get treat any
// CouchDB "not_found" as ErrNotFound (it currently only handles
// no_db_file / "Database does not exist."). That would remove the implicit
// dependency for every package, not just this one. The repercussions on
// other Get callers haven't been fully audited though, so this localized
// bootstrap stays in place until that broader change is vetted.
func TestMain(m *testing.M) {
	if err := loadTestConfigForMain(); err != nil {
		log.Fatalf("rabbitmq tests: could not load test config: %s", err)
	}
	// Best-effort: if CouchDB is unreachable, individual tests that need it
	// will fail via testutils.NeedCouchdb(t). We don't want TestMain itself
	// to hard-fail in environments where CouchDB is intentionally absent.
	if _, err := couchdb.CheckStatus(context.Background()); err == nil {
		if err := couchdb.InitGlobalDB(context.Background()); err != nil {
			log.Printf("rabbitmq tests: could not init global CouchDB: %s", err)
		}
	}
	os.Exit(m.Run())
}

// loadTestConfigForMain mirrors config.UseTestFile but without the
// *testing.T plumbing so it can be called from TestMain.
func loadTestConfigForMain() error {
	v := viper.New()
	v.SetConfigName("cozy.test")
	v.AddConfigPath("$HOME/.cozy")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetEnvPrefix("cozy")
	v.AutomaticEnv()
	v.SetDefault("host", "localhost")
	v.SetDefault("port", 8080)
	v.SetDefault("assets", "./assets")
	v.SetDefault("subdomains", "nested")
	v.SetDefault("fs.url", "mem://test")
	v.SetDefault("couchdb.url", "http://localhost:5984/")
	v.SetDefault("log.level", "info")
	if err := v.ReadInConfig(); err != nil {
		if _, isMissing := err.(viper.ConfigFileNotFoundError); !isMissing {
			return err
		}
	}
	return config.UseViper(v)
}
