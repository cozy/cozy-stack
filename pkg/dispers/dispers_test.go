package enclave

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
General tests on DISPERS API. HTTP requests are sent and answers are analysed.
*/

func TestDecrypteConcept(t *testing.T) {
	testCI := Actor{
		host: "localhost:8080",
		api:  "conceptindexor",
	}
	testCI.makeRequestPost("hash/concept=lib", "")
	assert.Equal(t, "foo", testCI.outstr)
}

func TestGetTargets(t *testing.T) {
	testTF := Actor{
		host: "localhost:8080",
		api:  "targetfinder",
	}
	testTF.makeRequestPost("adresses", "{ \"concepts\" : [ { \"adresses\" : [\"avr\", \"mai\"] } , {\"adresses\" : [\"hey\", \"oh\"] }, { \"adresses\" : [\"bla\", \"bla\"] } ] }")
	assert.Equal(t, "foo", testTF.outstr)
}

func TestGetTokens(t *testing.T) {
	testT := Actor{
		host: "localhost:8080",
		api:  "target",
	}
	testT.makeRequestPost("gettokens", "{ \"localquery\" : \"blafjiejfi\", \"adresses\" : [ \"abc\", \"iji\", \"jio\" ] }")
	assert.Equal(t, "foo", testT.outstr)

}

func TestGetData(t *testing.T) {
}

func TestAggregate(t *testing.T) {
	// Get Data From dummy_dataset
	s := ""
	absPath, _ := filepath.Abs("../cozy-stack/assets/test/dummy_dataset.json")
	buf, err := ioutil.ReadFile(absPath)
	if err == nil {
		s = string(buf)
	} else {
		fmt.Println(err)
	}

	// Launch Test On Aggregation
	testDA := Actor{
		host: "localhost:8080",
		api:  "dataaggregator",
	}
	testDA.makeRequestPost("aggregate", strings.Join([]string{"{ \"type\" : { \"dataset\" : \"bank.lib\", \"preprocess\" : \"tf-idf\", \"standardization\" : \"None\", \"shape\" : [20000, 1], \"fakelabels\" : [ \"X1\", \"X2\" ] } , \"data\" : \"", s, "\" }"}, ""))
	assert.Equal(t, "foo", testDA.outstr)
}

func TestUpdateDoc(t *testing.T) {
}

func TestLead(t *testing.T) {
}
