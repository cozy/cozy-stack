package dispers

import (
    "github.com/cozy/cozy-stack/pkg/couchdb"
    "github.com/cozy/cozy-stack/pkg/prefixer"
)

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

func GetSubscriptions(domain, prefix string) []string {

    // check if db exists in user's db
    mPrefixer := prefixer.NewPrefixer(domain, prefix)
    couchdb.EnsureDBExist(mPrefixer, "io.cozy.shared4ml")

    fetched := &SubscribeDoc{}
    err := couchdb.GetDoc(mPrefixer, "io.cozy.shared4ml", "subscription", fetched)
    if err != nil {
        return fetched.Subscriptions
    }
    return []string {"iris","lib.bank"}
}

func Subscribe(domain, prefix string, concepts []string){

    doc := &SubscribeDoc{
      SubscribeID: "subscription",
  		Subscriptions: concepts,
  	}

    mPrefixer := prefixer.NewPrefixer(domain, prefix)
    couchdb.CreateNamedDocWithDB(mPrefixer, doc)

}
