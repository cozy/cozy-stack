package data

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/stretchr/testify/assert"
)

func TestReplicationFromcozy(t *testing.T) {
	assert.NoError(t, couchdb.ResetDB(testInstance, Type))
	// creates 3 docs
	var doc1 = getDocForTest()
	var _ = getDocForTest()
	var _ = getDocForTest()

	var source = ts.URL + "/data/" + Type
	var target = config.CouchURL().String() + "replication-target"
	var replicator = config.CouchURL().String() + "_replicate"

	source = strings.Replace(source, "http://", "http://user:"+token+"@", 1)

	req, _ := http.NewRequest("DELETE", target, nil)
	doRequest(req, nil)
	// err is expected

	req, _ = http.NewRequest("PUT", target, nil)
	_, _, err := doRequest(req, nil)
	assert.NoError(t, err)

	req, _ = http.NewRequest("POST", replicator, jsonReader(&map[string]interface{}{
		"source": source,
		"target": target,
	}))
	req.Header.Add("Content-Type", "application/json")
	_, _, err = doRequest(req, nil)
	assert.NoError(t, err)

	req, _ = http.NewRequest("GET", target+"/"+doc1.ID(), nil)
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, out["_id"], doc1.ID())

	// add more docs, including a _design doc
	var doc4 = getDocForTest()
	err = couchdb.DefineIndex(testInstance, mango.IndexOnFields(Type, "my-index", []string{"test"}))
	assert.NoError(t, err)

	// replicate again
	req, _ = http.NewRequest("POST", replicator, jsonReader(&map[string]interface{}{
		"source": source,
		"target": target,
	}))
	req.Header.Add("Content-Type", "application/json")
	_, _, err = doRequest(req, nil)
	assert.NoError(t, err)

	req, _ = http.NewRequest("GET", target+"/"+doc4.ID(), nil)
	out, res, err = doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Equal(t, out["_id"], doc4.ID())
}
