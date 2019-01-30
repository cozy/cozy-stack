package sessions

import (
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
)

var JWTSecret = []byte("foobar")

func TestMain(m *testing.M) {
	delegatedInst = &instance.Instance{Domain: "external.notmycozy.com"}
	config.UseTestFile()
	conf := config.GetConfig()
	confAuth := make(map[string]interface{})
	fmt.Println(">>>>>>>> conf", conf)
	conf.Authentication = make(map[string]interface{})
	conf.Authentication[config.DefaultInstanceContext] = confAuth
	confAuth["jwt_secret"] = base64.StdEncoding.EncodeToString(JWTSecret)

	config.UseTestFile()
	os.Exit(m.Run())
}
