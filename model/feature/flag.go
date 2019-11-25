package feature

import (
	"encoding/json"
	"fmt"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

type Flags struct {
	DocID   string
	DocRev  string
	M       map[string]interface{}
	Sources []*Flags
}

func (f *Flags) ID() string        { return f.DocID }
func (f *Flags) Rev() string       { return f.DocRev }
func (f *Flags) DocType() string   { return consts.Settings }
func (f *Flags) SetID(id string)   { f.DocID = id }
func (f *Flags) SetRev(rev string) { f.DocRev = rev }
func (f *Flags) Clone() couchdb.Doc {
	clone := Flags{DocID: f.DocID, DocRev: f.DocRev}
	for k, v := range f.M {
		clone.M[k] = v
	}
	return &clone
}
func (f *Flags) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.M)
}

func GetFlags(inst *instance.Instance) (*Flags, error) {
	sources := make([]*Flags, 0)
	m := make(map[string]interface{})
	flags := &Flags{
		DocID:   consts.FlagsSettingsID,
		M:       m,
		Sources: sources,
	}
	if err := flags.addContext(inst); err != nil {
		return nil, err
	}
	return flags, nil
}

func (f *Flags) addContext(inst *instance.Instance) error {
	ctx, err := inst.SettingsContext()
	if err == instance.ErrContextNotFound {
		return nil
	} else if err != nil {
		return err
	}
	m, ok := ctx["features"].(map[interface{}]interface{})
	if !ok {
		return nil
	}
	normalized := make(map[string]interface{})
	for k, v := range m {
		normalized[fmt.Sprintf("%v", k)] = v
	}
	ctxFlags := &Flags{
		DocID: consts.ContextFlagsSettingsID,
		M:     normalized,
	}
	f.Sources = append(f.Sources, ctxFlags)
	for k, v := range ctxFlags.M {
		f.M[k] = v
	}
	return nil
}

var _ couchdb.Doc = &Flags{}
