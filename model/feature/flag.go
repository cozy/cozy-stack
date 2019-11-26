package feature

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
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
	clone.M = make(map[string]interface{})
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
	flags.addInstanceFlags(inst)
	if err := flags.addManager(inst); err != nil {
		return nil, err
	}
	if err := flags.addContext(inst); err != nil {
		return nil, err
	}
	if err := flags.addDefaults(inst); err != nil {
		return nil, err
	}
	return flags, nil
}

func (f *Flags) addInstanceFlags(inst *instance.Instance) {
	if len(inst.FeatureFlags) == 0 {
		return
	}
	m := make(map[string]interface{})
	for k, v := range inst.FeatureFlags {
		m[k] = v
	}
	flags := &Flags{
		DocID: consts.InstanceFlagsSettingsID,
		M:     m,
	}
	f.Sources = append(f.Sources, flags)
	for k, v := range flags.M {
		if _, ok := f.M[k]; !ok {
			f.M[k] = v
		}
	}
}

func (f *Flags) addManager(inst *instance.Instance) error {
	if len(inst.FeatureSets) == 0 {
		return nil
	}
	m, err := getFlagsFromManager(inst)
	if err != nil || len(m) == 0 {
		return err
	}
	flags := &Flags{
		DocID: consts.ManagerFlagsSettingsID,
		M:     m,
	}
	f.Sources = append(f.Sources, flags)
	for k, v := range flags.M {
		if _, ok := f.M[k]; !ok {
			f.M[k] = v
		}
	}
	return nil
}

var (
	cacheDuration      = 12 * time.Hour
	managerHTTPClient  = &http.Client{Timeout: 5 * time.Second}
	errInvalidResponse = errors.New("Invalid response from the manager")
)

func getFlagsFromManager(inst *instance.Instance) (map[string]interface{}, error) {
	cache := config.GetConfig().CacheStorage
	cacheKey := fmt.Sprintf("flags:%s:%v", inst.ContextName, inst.FeatureSets)
	var flags map[string]interface{}
	if buf, ok := cache.Get(cacheKey); ok {
		if err := json.Unmarshal(buf, &flags); err == nil {
			return flags, nil
		}
	}

	managerURL, err := inst.ManagerURL(instance.ManagerFeatureSetsURL)
	if err != nil {
		return nil, err
	}
	if managerURL == "" {
		return flags, nil
	}
	query := url.Values{
		"sets":    {strings.Join(inst.FeatureSets, ",")},
		"context": {inst.ContextName},
	}.Encode()
	res, err := managerHTTPClient.Get(fmt.Sprintf("%s?%s", managerURL, query))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errInvalidResponse
	}
	var data map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}
	var ok bool
	if flags, ok = data["flags"].(map[string]interface{}); !ok {
		return nil, errInvalidResponse
	}

	if buf, err := json.Marshal(flags); err == nil {
		cache.Set(cacheKey, buf, cacheDuration)
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
		if _, ok := f.M[k]; !ok {
			f.M[k] = v
		}
	}
	return nil
}

func (f *Flags) addDefaults(inst *instance.Instance) error {
	var defaults Flags
	err := couchdb.GetDoc(couchdb.GlobalDB, consts.Settings, consts.DefaultFlagsSettingsID, &defaults)
	if couchdb.IsNotFoundError(err) {
		return nil
	} else if err != nil {
		return err
	}
	defaults.SetID(consts.DefaultFlagsSettingsID)
	f.Sources = append(f.Sources, &defaults)
	for k, v := range defaults.M {
		if _, ok := f.M[k]; !ok {
			f.M[k] = v
		}
	}
	return nil
}

var _ couchdb.Doc = &Flags{}
