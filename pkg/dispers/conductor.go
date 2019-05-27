/*
In this file, we lead a DISPERS-ML learning. We're going to choose the Actors in
the process and talk with them. We're probabliy going to interact with several
stacks and several servers. This script is also going to keep the querier (front
-end) acknowledge of the process by updating repeatedly the doc in his Couchdb.
A Conductor is instanciated when a user call the associated route of this role.
*/

package enclave

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/echo"
)

/*
SubscribeDoc is used to save in Conductor's database the list of instances that subscribed
to a concept
*/
type SubscribeDoc struct {
	SubscribeID  string `json:"_id,omitempty"`
	SubscribeRev string `json:"_rev,omitempty"`
	Adresses     string `json:"adresses"`
}

/*
ID is used to get SubscribeID
*/
func (t *SubscribeDoc) ID() string {
	return t.SubscribeID
}

/*
Rev is used to get SubscribeRev
*/
func (t *SubscribeDoc) Rev() string {
	return t.SubscribeRev
}

/*
DocType is used to get the doc's type
*/
func (t *SubscribeDoc) DocType() string {
	return "io.cozy.shared4ml"
}

/*
Clone is used to copy one doc
*/
func (t *SubscribeDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

/*
SetID is used to set doc's ID
*/
func (t *SubscribeDoc) SetID(id string) {
	t.SubscribeID = id
}

/*
SetRev is used to set doc's Rev
*/
func (t *SubscribeDoc) SetRev(rev string) {
	t.SubscribeRev = rev
}

/*
Subscribe is used by a user to share a new data
*/
func Subscribe(domain, prefix string, adresses []string) {
	/*
		  couchdb.EnsureDBExist(prefixer.ConductorPrefixer, "io.cozy.shared4ml")

		  doc := &SubscribeDoc{
		    SubscribeID: "subscription",
				Adresses: adresses,
			}

		  couchdb.CreateNamedDocWithDB(mPrefixer, doc)
		  // TO DO : update doc in Conductor/shared4ml
	*/
}

/*
Actor structure gives the Conductor a way to consider every distant Actors
and to communicate with it.
*/
type Actor struct {
	host    string
	role    string
	outstr  string
	out     []byte
	outmeta string
}

func (a *Actor) makeRequestGet(job string) (dispers.Metadata, error) {

	url := ""

	if job == "" {
		url = strings.Join([]string{"http:/", a.host, "dispers", a.role}, "/")
	} else {
		url = strings.Join([]string{"http:/", a.host, "dispers", a.role, job}, "/")
	}

	meta := dispers.NewMetadata("HTTP Get", url, []string{"HTTP", "CI"})

	resp, err := http.Get(url)
	if err != nil {
		meta.Close("", err)
		return meta, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		meta.Close("", err)
		return meta, err
	}

	a.outstr = string(body)
	a.out = body
	meta.Close(a.outstr, err)
	return meta, nil

}

func (a *Actor) makeRequestPost(job string, data string) (dispers.Metadata, error) {

	url := strings.Join([]string{"http://", a.host, "/dispers/", a.role, "/", job}, "")

	meta := dispers.NewMetadata("HTTP Post", url, []string{"HTTP", "CI"})

	resp, err := http.Post(url, "application/json", bytes.NewBufferString(data))
	if err != nil {
		meta.Close("", err)
		return meta, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		meta.Close("", err)
		return meta, err
	}

	a.outstr = string(body)
	a.out = body
	meta.Close(a.outstr, err)
	return meta, nil

}

// makeRequestPatch

func (a *Actor) makeRequestDelete(job string) (dispers.Metadata, error) {
	// Create client
	client := &http.Client{}

	// Create url
	url := ""
	if job == "" {
		url = strings.Join([]string{"http:/", a.host, "dispers", a.role}, "/")
	} else {
		url = strings.Join([]string{"http:/", a.host, "dispers", a.role, job}, "/")
	}

	meta := dispers.NewMetadata("HTTP Post", url, []string{"HTTP", "CI"})

	// Create request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		meta.Close("", err)
		return meta, err
	}

	// Fetch Request
	resp, err := client.Do(req)
	if err != nil {
		meta.Close("", err)
		return meta, err
	}
	defer resp.Body.Close()

	// Read Response Body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		meta.Close("", err)
		return meta, err
	}

	a.outstr = string(respBody)
	a.out = respBody
	meta.Close(a.outstr, err)
	return meta, nil
}

/*
The script is going to retrieve informations in the querier's db and follows
this informations to the different role. The most part of this informations is
encrypted, the Conductor is not supposed to deduced anything from all what he is
manipulating.
*/

/*
Every training has metadata saved in the Conductor's database. Thanks to that,
the querier can retrieve the learning's state.
*/
type queryDoc struct {
	QueryID                    string           `json:"_id,omitempty"`
	QueryRev                   string           `json:"_rev,omitempty"`
	Metadata                   dispers.Metadata `json:"metadata,omitempty"`
	EncryptedLocalQuery        []byte           `json:"localquery,omitempty"`
	EncryptedAggregateFunction []byte           `json:"aggrfunc,omitempty"`
	EncryptedConcepts          []byte           `json:"concepts,omitempty"`
	EncryptedTargetProfile     []byte           `json:"targetprofile,omitempty"`
	Results                    []byte           `json:"results,omitempty"`
}

func (t *queryDoc) ID() string {
	return t.QueryID
}

func (t *queryDoc) Rev() string {
	return t.QueryRev
}

func (t *queryDoc) DocType() string {
	return "io.cozy.ml"
}

func (t *queryDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *queryDoc) SetID(id string) {
	t.QueryID = id
}

func (t *queryDoc) SetRev(rev string) {
	t.QueryRev = rev
}

// GetTrainingState can be called by the querier to have some information about
// the process. GetTrainingState send information from the Conductor's database
func GetTrainingState(id string) echo.Map {
	couchdb.EnsureDBExist(prefixer.ConductorPrefixer, "io.cozy.ml")
	fetched := &queryDoc{}
	err := couchdb.GetDoc(prefixer.ConductorPrefixer, "io.cozy.ml", id, fetched)
	if err != nil {
		return echo.Map{"outcome": "error",
			"message": err}
	}

	metas, err := dispers.RetrieveMetadata(id)
	if err != nil {
		return echo.Map{"outcome": "error",
			"message": err}
	}

	return echo.Map{"outcome": "ok",
		"training": fetched.Results,
		"metadata": metas,
	}
}

/*
In order to handle several layers of DA, we create a structure called AggregationLayer
It is pretty much the same than layers in Neural Networks.
type aggregationLayer struct {
  input            string
  output           string
  unit             int16
  process          string
  dataaggregators  []Actor
}
*/

// Conductor collects every actors playing a part to the query
type Conductor struct {
	Doc                queryDoc
	State              string
	Targetfinders      []Actor
	Conceptindexors    []Actor
	Datas              []Actor
	Dataaggregators    []Actor
	Maindataaggregator []Actor
	/*StackAggr           []aggregationLayer*/
	LocalQuery        []byte
	AggregateFunction []byte
	Concepts          [][]byte
	TargetProfile     []byte
}

/*
NewConductor returns a Conductor object with the specified values.
This object will be created directly in the cmd shell / web role
This object use the major part of what have been created before in this script
*/
func NewConductor(domain, prefix string, encLocalQuery []byte, encAggregateFunction []byte, encConcepts [][]byte, encTargetProfile []byte) (*Conductor, error) {

	// TODO: Initiate cleanly actors from a list of hosts
	firstCI := Actor{
		host: "localhost:8080",
		role: "conceptindexor",
	}

	querydoc := queryDoc{
		QueryID:  "",
		QueryRev: "",
	}

	if err := couchdb.CreateDoc(prefixer.ConductorPrefixer, &querydoc); err != nil {
		return &Conductor{}, err
	}

	// Doc's creation in CouchDB
	couchdb.EnsureDBExist(prefixer.DataAggregatorPrefixer, "io.cozy.aggregation")

	// récupérer l'id du doc sur prefix/io.cozy.ml
	docID := "17f78f7e8f7484z6"
	docRev := "2-46148"

	retour := &Conductor{
		Doc: queryDoc{
			QueryID:  docID,
			QueryRev: docRev,
		},
		Conceptindexors:   []Actor{firstCI},
		State:             "Training",
		LocalQuery:        encLocalQuery,
		AggregateFunction: encAggregateFunction,
		Concepts:          encConcepts,
		TargetProfile:     encTargetProfile,
	}

	return retour, nil
}

func getListOfInstances(element string) ([]byte, error) {
	return []byte(""), nil
}

// DecrypteConcept returns a list of hashed concepts from a list of encrypted concepts
func (c *Conductor) DecrypteConcept(encryptedConcepts [][]byte) ([]string, error) {

	meta := dispers.NewMetadata("Decrypt Concept", strconv.Itoa(len(encryptedConcepts)), []string{"Conductor", "CI"})

	// Call role-CI for each concept
	hashedConcepts := make([]string, len(encryptedConcepts))
	for index, element := range encryptedConcepts {
		metaReq, err := c.Conceptindexors[0].makeRequestPost(strings.Join([]string{"hash/concept=", string(element)}, ""), "")
		if err != nil {
			meta.Close("", err)
			meta.Push(c.Doc.QueryID)
			return []string{}, err
		}

		metaReq.Push(c.Doc.QueryID)
		var outputci dispers.OutputCI
		json.Unmarshal(c.Conceptindexors[0].out, &outputci)
		hashedConcepts[index] = outputci.Hash
	}

	meta.Close(strings.Join(hashedConcepts, " - "), nil)
	meta.Push(c.Doc.QueryID)
	return hashedConcepts, nil
}

// GetTargets works with TF's role
func (c *Conductor) GetTargets(encryptedLists [][]byte) ([]byte, error) {
	return []byte(""), nil
}

// GetTokens works with T's role
func (c *Conductor) GetTokens(finalListe []byte) []dispers.Metadata {
	return []dispers.Metadata{}
}

// GetData works with stacks
func (c *Conductor) GetData() (string, error) {
	s := ""
	return s, nil
}

// Aggregate works with DA's role
func (c *Conductor) Aggregate() error {
	return nil
}

// UpdateDoc is used to add a metadata to the Query Doc so that the querier is able to know the state of his training
func (c *Conductor) UpdateDoc(key string, metadatas []dispers.Metadata) error { return nil }

// Lead is the most general method. This is the only one used in CMD and Web's files. It will use the 5 previous methods to work
func (c *Conductor) Lead() error {

	hashedConcepts, err := c.DecrypteConcept(c.Concepts)
	if err != nil {
		return err
	}

	// Retrieve associated list for each concept
	encryptedLists := make([][]byte, len(hashedConcepts))
	for index, element := range hashedConcepts {
		encryptedLists[index], err = getListOfInstances(element)
		if err != nil {
			return err
		}
	}

	finalList, err := c.GetTargets(encryptedLists)
	if err != nil {
		return err
	}

	_ = c.GetTokens(finalList)
	if err != nil {
		return err
	}

	data := ""
	data, _ = c.GetData()
	strings.ToLower(data) // juste pour éviter l'erreur du "data is not used"
	if err != nil {
		return err
	}

	// TODO: Deal with Async Method
	_ = c.Aggregate()
	if err != nil {
		return err
	}

	// TODO: Send the result to the Querier's stack on a dedicated routes

	return nil
}
