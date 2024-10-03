package rag

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

func AddDescriptionToFile(inst *instance.Instance, file *vfs.FileDoc) (*vfs.FileDoc, error) {
	content, err := inst.VFS().OpenFile(file)
	if err != nil {
		return nil, err
	}
	defer content.Close()
	description, err := callRAGDescription(inst, content, file.Mime)
	if err != nil {
		return nil, err
	}
	newfile := file.Clone().(*vfs.FileDoc)
	if newfile.Metadata == nil {
		newfile.Metadata = make(map[string]interface{})
	}
	newfile.Metadata["description"] = description
	if err := couchdb.UpdateDocWithOld(inst, newfile, file); err != nil {
		return nil, err
	}
	return newfile, nil
}

func callRAGDescription(inst *instance.Instance, content io.Reader, mime string) (string, error) {
	ragServer := inst.RAGServer()
	if ragServer.URL == "" {
		return "", errors.New("no RAG server configured")
	}
	u, err := url.Parse(ragServer.URL)
	if err != nil {
		return "", err
	}
	u.Path = "/description"
	req, err := http.NewRequest(http.MethodPost, u.String(), content)
	if err != nil {
		return "", err
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+ragServer.APIKey)
	req.Header.Add("Content-Type", mime)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("POST status code: %d", res.StatusCode)
	}
	var data struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return "", err
	}
	if data.Description == "" {
		return "", errors.New("no description")
	}
	return data.Description, nil
}
