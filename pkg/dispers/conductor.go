package dispers

import (
  	"github.com/cozy/echo"
    "github.com/cozy/cozy-stack/pkg/couchdb"
    "github.com/cozy/cozy-stack/pkg/prefixer"
    "github.com/cozy/cozy-stack/pkg/dispers/utils"
)

// In this file, we lead a DISPERS-ML learning. We're going to choose the actors in the process. We're probabliy going to interact with several stacks in several servers
// This script is also going to keep the querier (front-end) acknowledge of the process by updating repeatedly the doc in his Couchdb
// To do all this task, we need to define several concepts :
// - SubscribeDoc : to make one capable of deciding if he wants to share its data
// - Actors : to work with several api
// - queryDoc : to update the initial doc made by tis querier in the cozy app
// - AggregationLayer : to add layers of DA


// In the first part of this script, we deal with the use-case : the user want to decide if he wants to share or not its data
// The list of cozys and the list subscritions are saved at different places (querier, api-ci) in different forms (transposed and encrypted)
type SubscribeDoc struct {
	SubscribeID  string `json:"_id,omitempty"`
	SubscribeRev string `json:"_rev,omitempty"`
	Subscriptions  []string `json:"subscriptions"`
}

func (t *SubscribeDoc) ID() string {
	return t.SubscribeID
}

func (t *SubscribeDoc) Rev() string {
	return t.SubscribeRev
}

func (t *SubscribeDoc) DocType() string {
	return "io.cozy.shared4ml"
}

func (t *SubscribeDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *SubscribeDoc) SetID(id string) {
	t.SubscribeID = id
}

func (t *SubscribeDoc) SetRev(rev string) {
	t.SubscribeRev = rev
}

// GetSubscriptions is used by a user to know what data he is sharing currently
func GetSubscriptions(domain, prefix string) []string {

    /*
    // check if db exists in user's db
    mPrefixer := prefixer.NewPrefixer(domain, prefix)
    couchdb.EnsureDBExist(mPrefixer, "io.cozy.shared4ml")
    couchdb.EnsureDBExist(prefixer.ConductorPrefixer, "io.cozy.shared4ml")

    fetched := &SubscribeDoc{}
    err := couchdb.GetDoc(mPrefixer, "io.cozy.shared4ml", "subscription", fetched)
    if err != nil {
        return fetched.Subscriptions
    }
    */

    return []string {"iris","lib.bank"}
}

// GetSubscriptions is used by a user to share a new data
func Subscribe(domain, prefix string, concepts []string){
    // check if db exists in user's db and in conductor's db
    mPrefixer := prefixer.NewPrefixer(domain, prefix)
    couchdb.EnsureDBExist(mPrefixer, "io.cozy.shared4ml")
    couchdb.EnsureDBExist(prefixer.ConductorPrefixer, "io.cozy.shared4ml")

    doc := &SubscribeDoc{
      SubscribeID: "subscription",
  		Subscriptions: concepts,
  	}

    couchdb.CreateNamedDocWithDB(mPrefixer, doc)
    // TO DO : update doc in conductor/shared4ml
}

// In this second part of the script, we are going to deal with the use-case : the user want to launch a ML Training
// The script is going to retrieve informations in the querier's db and follow this informations to the different api
// The most part of this informations is encrypted, the conductor is not supposed to deduced so much from all what he is manipulating.
// A Training has the informations relatives to a Training.
type Training struct {
  AlgoML         string   `json:"algo,omitempty"` // model trained
  Dataset        string   `json:"dataset,omitempty"` // dataset used
  DispersAlgo    string   `json:"dispersalgo,omitempty"` // typo from dispers-ml
  FormulaTarget  string   `json:"formulatarget,omitempty"` // var from dataset to predict
  FormulaPreds   []string `json:"formulapredictors,omitempty"` // predictors from dataset
  State          string   `json:"state,omitempty"`
  // parameters
}

type queryDoc struct {
	queryID     string         `json:"_id,omitempty"`
	queryRev    string         `json:"_rev,omitempty"`
  MyTraining  Training       `json:"training,omitempty"`
  MyMetada    utils.Metadata `json:"metadata,omitempty"`
}

func (t *queryDoc) ID() string {
	return t.queryID
}

func (t *queryDoc) Rev() string {
	return t.queryRev
}

func (t *queryDoc) DocType() string {
	return "io.cozy.ml"
}

func (t *queryDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *queryDoc) SetID(id string) {
	t.queryID = id
}

func (t *queryDoc) SetRev(rev string) {
	t.queryRev = rev
}

func NewQueryDoc(queryID string, queryRev string, MyTraining Training, MyMetada utils.Metadata) *queryDoc {
	return &queryDoc{
    queryID: queryID,
    queryRev: queryRev,
    MyTraining: MyTraining,
    MyMetada: MyMetada,
    }
}

type aggregationLayer struct {
  input            string
  output           string
  unit             int16
  process          string
  dataaggregators  []utils.Actor
}

type conductor struct {
  doc                 queryDoc // Doc in the querier's database where are saved parameters, metadata and results
  mPrefixer           prefixer.Prefixer
  targetfinders       []utils.Actor
  conceptindexors     []utils.Actor
  datas               []utils.Actor
  dataaggregators     []utils.Actor
  maindataaggregator  []utils.Actor
  MyTraining           Training
  stackAggr           []aggregationLayer
}

// NewConductor returns a Conductor object with the specified values.
// This object will be created directly in the cmd shell / web api
// This object use the major part of what have been created before in this script
func NewConductor(domain, prefix string) *conductor {
  pref := prefixer.NewPrefixer(domain, prefix)
  // récupérer l'id du doc sur prefix/io.cozy.ml
  doc_id := "17f78f7e8f7484z6"
  doc_rev := "2-46148"

	return &conductor{
    mPrefixer: pref,
    doc: queryDoc{
      queryID: doc_id,
      queryRev: doc_rev,
    },
  }
}

func GetTrainingState(id string) echo.Map {
  couchdb.EnsureDBExist(prefixer.ConductorPrefixer, "io.cozy.ml")
  fetched := &queryDoc{}
  err := couchdb.GetDoc(prefixer.ConductorPrefixer, "io.cozy.ml", id, fetched)
  if err != nil {
    return echo.Map{"outcome": "error",
                  "message": err }
  }
  return echo.Map{"outcome": "ok",
                  "training" : fetched.MyTraining,
                  "metadata" : fetched.MyMetada}
}

func (c *conductor) DecrypteConcept() utils.Metadata { return nil }

func (c *conductor) ReachTargets() utils.Metadata { return nil }

func (c *conductor) GetTrain() utils.Metadata { return nil }

func (c *conductor) Aggregate() utils.Metadata { return nil }

func (c *conductor) UpdateDoc(key string, metadata utils.Metadata) error { return nil }

// This method is the most general. This is the only one used in CMD and Web's files. It will use the 5 previous methods to work
func (c *conductor) Lead() error {
  /*
  tempMetadata := utils.NewMetadata("aujourd'hui", true)
  UpdateDoc("meta-task-0-init", tempMetadata)

  if (tempMetadata.Outcome()){
    tempMetadata = DecrypteConcept()
    UpdateDoc("meta-task-1-ci", tempMetadata)
  }

  if (tempMetadata.Outcome()){
    tempMetadata = ReachTargets()
    UpdateDoc("meta-task-2-tf", tempMetadata)
  }

  if (tempMetadata.Outcome()){
    tempMetadata = GetData()
    UpdateDoc("meta-task-3-d", tempMetadata)
  }

  if (tempMetadata.Outcome()){
    tempMetadata = Aggregate()
    UpdateDoc("meta-task-4-da", tempMetadata)
  }
*/

  return nil

}
