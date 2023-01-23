package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/client/request"

	"github.com/nightlyone/lockfile"
)

// TokenFileFmt is the filename in which are stored OAuth client data and token.
const TokenFileFmt = ".cozy-oauth-%s"

// Storage is an interface to specify how to store and load authentication
// states.
type Storage interface {
	Load(domain string) (client *Client, token *AccessToken, err error)
	Save(domain string, client *Client, token *AccessToken) error
}

// FileStorage implements the Storage interface using a simple file.
type FileStorage struct{}

type authData struct {
	Client *Client      `json:"client,omitempty"`
	Token  *AccessToken `json:"token,omitempty"`
	Domain string       `json:"domain,omitempty"`
}

// NewFileStorage creates a new *FileStorage
func NewFileStorage() *FileStorage {
	return &FileStorage{}
}

// Load reads from the OAuth file and the states stored for the specified
// domain.
func (s *FileStorage) Load(domain string) (client *Client, token *AccessToken, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	filename := filepath.Join(homeDir, fmt.Sprintf(TokenFileFmt, domain))
	l, err := newFileLock(filename)
	if err != nil {
		return nil, nil, err
	}
	if err = l.TryLock(); err != nil {
		return nil, nil, err
	}
	defer func() {
		if err := l.Unlock(); err != nil {
			fmt.Fprintf(os.Stderr, "Error on unlock: %s", err)
		}
	}()
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, io.EOF) {
			err = nil
		}
		return nil, nil, err
	}
	data := &authData{}
	if err = request.ReadJSON(f, data); err != nil {
		fmt.Fprintf(os.Stderr, "Authentication file %s is malformed: %s",
			filename, err.Error())
		return nil, nil, nil
	}
	if data.Domain != domain {
		return nil, nil, err
	}
	return data.Client, data.Token, nil
}

// Save writes the authentication states to a file for the specified domain.
func (s *FileStorage) Save(domain string, client *Client, token *AccessToken) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	filename := filepath.Join(homeDir, fmt.Sprintf(TokenFileFmt, domain))
	l, err := newFileLock(filename)
	if err != nil {
		return err
	}
	if err = l.TryLock(); err != nil {
		return err
	}
	defer func() {
		if err := l.Unlock(); err != nil {
			fmt.Fprintf(os.Stderr, "Error on unlock: %s", err)
		}
	}()
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	data := &authData{
		Client: client,
		Token:  token,
		Domain: domain,
	}
	return json.NewEncoder(f).Encode(data)
}

func newFileLock(name string) (lockfile.Lockfile, error) {
	lockName := strings.ReplaceAll(name, "/", "_") + ".lock"
	return lockfile.New(filepath.Join(os.TempDir(), lockName))
}
