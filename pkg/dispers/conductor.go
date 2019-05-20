/*
In this file, we lead a DISPERS-ML learning. We're going to choose the Actors in
the process and talk with them. We're probabliy going to interact with several
stacks and several servers. This script is also going to keep the querier (front
-end) acknowledge of the process by updating repeatedly the doc in his Couchdb.
A Conductor is instanciated when a user call the associated route of this API.
*/

package enclave

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
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
	api     string
	outstr  string
	out     map[string]interface{}
	outmeta string
}

func (a *Actor) makeRequestGet(job string) error {

	url := ""

	if job == "" {
		url = strings.Join([]string{"http:/", a.host, "dispers", a.api}, "/")
	} else {
		url = strings.Join([]string{"http:/", a.host, "dispers", a.api, job}, "/")
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	a.outstr = string(body)
	json.NewDecoder(bytes.NewReader(body)).Decode(&a.out)
	return nil

}

func (a *Actor) makeRequestPost(job string, data string) error {

	url := strings.Join([]string{"http://", a.host, "/dispers/", a.api, "/", job}, "")

	resp, err := http.Post(url, "application/json", bytes.NewBufferString(data))
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	a.outstr = string(body)
	json.NewDecoder(bytes.NewReader(body)).Decode(&a.out)
	return nil

}

// makeRequestPatch

// makeRequestDelete

/*
The script is going to retrieve informations in the querier's db and follows
this informations to the different api. The most part of this informations is
encrypted, the Conductor is not supposed to deduced anything from all what he is
manipulating.
*/

/*
Training structure pack every information for one training
*/
type Training struct {
	AlgoML        string   `json:"algo,omitempty"`              // model trained
	Dataset       string   `json:"dataset,omitempty"`           // dataset used
	DispersAlgo   string   `json:"dispersalgo,omitempty"`       // typo from dispers-ml
	FormulaTarget string   `json:"formulatarget,omitempty"`     // var from dataset to predict
	FormulaPreds  []string `json:"formulapredictors,omitempty"` // predictors from dataset
	State         string   `json:"state,omitempty"`
	// parameters
}

/*
Every training has metadata saved in the Conductor's database. Thanks to that,
the querier can retrieve the learning's state.
*/
type queryDoc struct {
	QueryID    string           `json:"_id,omitempty"`
	QueryRev   string           `json:"_rev,omitempty"`
	MyTraining Training         `json:"training,omitempty"`
	MyMetada   dispers.Metadata `json:"metadata,omitempty"` // A changer pour différencier chaque acteur
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

/*
NewQueryDoc is used to initiate a QueryDoc
*/
func newQueryDoc(MyTraining Training, MyMetada dispers.Metadata) *queryDoc {
	return &queryDoc{
		QueryID:    "",
		QueryRev:   "",
		MyTraining: MyTraining,
		MyMetada:   MyMetada,
	}
}

func GetTrainingState(id string) echo.Map {
	couchdb.EnsureDBExist(prefixer.ConductorPrefixer, "io.cozy.ml")
	fetched := &queryDoc{}
	err := couchdb.GetDoc(prefixer.ConductorPrefixer, "io.cozy.ml", id, fetched)
	if err != nil {
		return echo.Map{"outcome": "error",
			"message": err}
	}
	return echo.Map{"outcome": "ok",
		"training": fetched.MyTraining,
		"metadata": fetched.MyMetada}
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

type Conductor struct {
	Doc                queryDoc // Doc in the querier's database where are saved parameters, metadata and results
	mPrefixer          prefixer.Prefixer
	targetfinders      []Actor
	conceptindexors    []Actor
	datas              []Actor
	dataaggregators    []Actor
	maindataaggregator []Actor
	MyTraining         Training
	/*stackAggr           []aggregationLayer*/
}

/*
NewConductor returns a Conductor object with the specified values.
This object will be created directly in the cmd shell / web api
This object use the major part of what have been created before in this script
*/
func NewConductor(domain, prefix string, mytraining Training) (*Conductor, error) {

	mytraining.State = "Training"
	querydoc := newQueryDoc(mytraining, dispers.NewMetadata("creation", "creation du training", "aujourd'hui", true))

	if err := couchdb.CreateDoc(prefixer.ConductorPrefixer, querydoc); err != nil {
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
	}

	return retour, nil
}

// Works with CI's API
func (c *Conductor) DecrypteConcept() dispers.Metadata {
	return dispers.NewMetadata("nom de la metadata", "description", "aujourd'hui à telle heure", true)

}

// Works with TF's API
func (c *Conductor) GetTargets() dispers.Metadata {
	return dispers.NewMetadata("nom de la metadata", "description", "aujourd'hui à telle heure", true)
}

// Works with T's API
func (c *Conductor) GetTokens() dispers.Metadata {
	return dispers.NewMetadata("nom de la metadata", "description", "aujourd'hui à telle heure", true)
}

// Works with stacks
func (c *Conductor) GetData() (string, dispers.Metadata) {
	s := ""
	return s, dispers.NewMetadata("nom de la metadata", "description", "aujourd'hui à telle heure", true)
}

// Works with DA's API
func (c *Conductor) Aggregate() dispers.Metadata {
	return dispers.NewMetadata("nom de la metadata", "description", "aujourd'hui à telle heure", true)
}

// This method is used to add a metadata to the Query Doc so that the querier is able to know the state of his training
func (c *Conductor) UpdateDoc(key string, metadata dispers.Metadata) error { return nil }

// This method is the most general. This is the only one used in CMD and Web's files. It will use the 5 previous methods to work
func (c *Conductor) Lead() error {

	tempMetadata := dispers.NewMetadata("nom de la metadata", "description", "aujourd'hui à telle heure", true)
	c.UpdateDoc("meta-task-0-init", tempMetadata)

	if tempMetadata.Outcome() {
		tempMetadata = c.DecrypteConcept()
		c.UpdateDoc("meta-task-1-ci", tempMetadata)
	}

	if tempMetadata.Outcome() {
		tempMetadata = c.GetTargets()
		c.UpdateDoc("meta-task-2-tf", tempMetadata)
	}

	if tempMetadata.Outcome() {
		tempMetadata = c.GetTokens()
		c.UpdateDoc("meta-task-3-t", tempMetadata)
	}

	if tempMetadata.Outcome() {
		data := ""
		data, tempMetadata = c.GetData()
		strings.ToLower(data) // juste pour éviter l'erreur du "data is not used"
		c.UpdateDoc("meta-task-4-slack", tempMetadata)
	}

	if tempMetadata.Outcome() {
		tempMetadata = c.Aggregate()
		c.UpdateDoc("meta-task-5-da", tempMetadata)
	}

	return nil
}
