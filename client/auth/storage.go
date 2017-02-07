package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/nightlyone/lockfile"
)

// TokenFileFmt is the filename in which are stored OAuth client data and token.
const TokenFileFmt = ".cozy-oauth-%s" // #nosec

// Storage is an interface to specify how to store and load authentication
// states.
type Storage interface {
	Load(domain string) (client *Client, token *AccessToken, state string, err error)
	Save(domain string, client *Client, token *AccessToken, state string) error
}

// FileStorage implements the Storage interface using a simple file.
type FileStorage struct{}

type authData struct {
	Client *Client      `json:"client,omitempty"`
	Token  *AccessToken `json:"token,omitempty"`
	State  string       `json:"state,omitempty"`
	Domain string       `json:"domain,omitempty"`
}

// NewFileStorage creates a new *FileStorage
func NewFileStorage() *FileStorage {
	return &FileStorage{}
}

// Load reads from the OAuth file and the states stored for the specified
// domain.
func (s *FileStorage) Load(domain string) (client *Client, token *AccessToken, state string, err error) {
	filename := filepath.Join(utils.UserHomeDir(), fmt.Sprintf(TokenFileFmt, domain))
	l, err := newFileLock(filename)
	if err != nil {
		return
	}
	if err = l.TryLock(); err != nil {
		return
	}
	defer l.Unlock()
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) || err == io.EOF {
			err = nil
		}
		return
	}
	data := &authData{}
	if err = request.ReadJSON(f, data); err != nil {
		err = nil
		return
	}
	if data.Domain != domain {
		return
	}
	client = data.Client
	token = data.Token
	state = data.State
	return
}

// Save writes the authentication states to a file for the specified domain.
func (s *FileStorage) Save(domain string, client *Client, token *AccessToken, state string) error {
	filename := filepath.Join(utils.UserHomeDir(), fmt.Sprintf(TokenFileFmt, domain))
	l, err := newFileLock(filename)
	if err != nil {
		return err
	}
	if err = l.TryLock(); err != nil {
		return err
	}
	defer l.Unlock()
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	data := &authData{
		Client: client,
		Token:  token,
		State:  state,
		Domain: domain,
	}
	return json.NewEncoder(f).Encode(data)
}

func newFileLock(name string) (lockfile.Lockfile, error) {
	lockName := strings.Replace(name, "/", "_", -1) + ".lock"
	return lockfile.New(filepath.Join(os.TempDir(), lockName))
}
