package sharings

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/labstack/gommon/log"
)

func init() {
	jobs.AddWorker("sharedata", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 3,
		Timeout:      10 * time.Second,
		WorkerFunc:   SendData,
	})
}

// SendOptions describes the parameters needed to send data
type SendOptions struct {
	DocID      string
	DocType    string
	Method     string
	Recipients []*RecipientInfo
}

// RecipientInfo describes the recipient information
type RecipientInfo struct {
	URL   string
	Token string
}

// SendData sends data to all the recipients
func SendData(ctx context.Context, m *jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)

	opts := &SendOptions{}
	err := m.Unmarshal(&opts)
	if err != nil {
		return err
	}
	if opts.DocType != consts.Files {
		if opts.Method == http.MethodDelete {
			return DeleteDoc(domain, opts)
		}
		return SendDoc(domain, opts)
	}
	return SendFile(domain, opts)
}

// DeleteDoc asks the recipients to delete the shared document which id was
// provided.
func DeleteDoc(domain string, opts *SendOptions) error {
	path := fmt.Sprintf("/sharings/doc/%s/%s", opts.DocType, opts.DocID)

	for _, rec := range opts.Recipients {
		rev, err := getDocRevAtRecipient(opts.DocType, opts.DocID, rec)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to send "+
				"update : ", err)
			continue
		}

		_, errSend := request.Req(&request.Options{
			Domain: rec.URL,
			Method: opts.Method,
			Path:   path,
			Headers: request.Headers{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer " + rec.Token,
			},
			Body:       nil,
			Queries:    url.Values{"rev": {rev}},
			NoResponse: true,
		})
		if errSend != nil {
			log.Error("[sharing] An error occurred while trying to share "+
				"data : ", errSend)
		}

	}

	return nil
}

// SendDoc sends a JSON document to the recipients
func SendDoc(domain string, opts *SendOptions) error {
	// Get the doc
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	doc := &couchdb.JSONDoc{}
	if err := couchdb.GetDoc(i, opts.DocType, opts.DocID, doc); err != nil {
		return err
	}
	// A new doc will be created on the recipient side
	if opts.Method == http.MethodPost {
		delete(doc.M, "_id")
		delete(doc.M, "_rev")
	}

	path := fmt.Sprintf("/sharings/doc/%s/%s", opts.DocType, opts.DocID)

	for _, rec := range opts.Recipients {
		// A doc update requires to set the doc revision from each recipient
		if opts.Method == http.MethodPut {
			rev, err := getDocRevAtRecipient(opts.DocType, opts.DocID, rec)
			if err != nil {
				log.Error("[sharing] An error occurred while trying to send "+
					"update : ", err)
				continue
			}
			doc.SetRev(rev)
		}
		body, err := request.WriteJSON(doc.M)
		if err != nil {
			return err
		}

		// Send the document to the recipient
		// TODO : handle send failures
		_, errSend := request.Req(&request.Options{
			Domain: rec.URL,
			Method: opts.Method,
			Path:   path,
			Headers: request.Headers{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer " + rec.Token,
			},
			Body:       body,
			NoResponse: true,
		})
		if errSend != nil {
			log.Error("[sharing] An error occurred while trying to share "+
				"data : ", errSend)
		}
	}
	return nil
}

// SendFile sends a binary file to the recipients
func SendFile(domain string, opts *SendOptions) error {

	// Particular case for the root directory: don't share it
	if opts.DocID == consts.RootDirID {
		return nil
	}

	// Get VFS reference from instance
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	if i == nil {
		log.Error("[sharing] An error occurred while trying to share " +
			"a file: instance not found")
		return nil
	}
	fs := i.VFS()

	// Get file doc
	doc, err := fs.FileByID(opts.DocID)
	if err != nil {
		return err
	}
	// Send references for permissions
	// TODO: only send the reference linked  to the actual permission
	b, err := json.Marshal(doc.ReferencedBy)
	if err != nil {
		return nil
	}
	refs := string(b[:])

	path := fmt.Sprintf("/sharings/doc/%s/%s", consts.Files, opts.DocID)
	query := url.Values{
		"Type":          []string{consts.FileType},
		"Name":          []string{doc.DocName},
		"Referenced_by": []string{refs},
	}
	md5 := base64.StdEncoding.EncodeToString(doc.MD5Sum)
	length := strconv.FormatInt(doc.ByteSize, 10)

	// Get file content
	content, err := fs.OpenFile(doc)
	if err != nil {
		return err
	}
	defer content.Close()

	for _, rec := range opts.Recipients {
		if err != nil {
			return err
		}

		_, errSend := request.Req(&request.Options{
			Domain: rec.URL,
			Method: opts.Method,
			Path:   path,
			Headers: request.Headers{
				"Content-Type":   doc.Mime,
				"Accept":         "application/vnd.api+json",
				"Content-Length": length,
				"Content-MD5":    md5,
				"Authorization":  "Bearer " + rec.Token,
			},
			Queries: query,
			Body:    content,
		})
		if errSend != nil {
			log.Error("[sharing] An error occurred while trying to share "+
				"file : ", errSend)
		}
	}

	return nil
}

// getDocRevAtRecipient returns the revision of the document at the given
// recipient.
func getDocRevAtRecipient(doctype, docID string, recInfo *RecipientInfo) (string, error) {
	path := fmt.Sprintf("/data/%s/%s", doctype, docID)

	res, err := request.Req(&request.Options{
		Domain: recInfo.URL,
		Method: "GET",
		Path:   path,
		Headers: request.Headers{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + recInfo.Token,
		},
	})
	if err != nil {
		return "", err
	}
	doc := &couchdb.JSONDoc{}
	if err := request.ReadJSON(res.Body, doc); err != nil {
		return "", err
	}
	rev := doc.M["_rev"].(string)
	return rev, nil
}
