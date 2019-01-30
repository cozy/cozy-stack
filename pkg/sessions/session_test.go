package sessions

import (
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
)

func TestMain(m *testing.M) {
	delegatedInst = &instance.Instance{Domain: "external.notmycozy.com"}
	config.UseTestFile()
	os.Exit(m.Run())
}
