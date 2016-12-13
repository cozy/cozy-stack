package data

import (
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/stretchr/testify/assert"
)

func TestReplicationFromcozy(t *testing.T) {
	assert.NoError(t, couchdb.ResetDB(testInstance, Type))
	// creates 3 docs
	var doc1 = getDocForTest()
	var _ = getDocForTest()
	var _ = getDocForTest()

	var source = ts.URL + "/data/" + Type
	var target = config.CouchURL() + "replication-target"
	var replicator = config.CouchURL() + "_replicate"

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
}
