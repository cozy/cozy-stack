package sharings

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
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

// RecipientInfo describes the recipient information
type RecipientInfo struct {
	URL    string
	Scheme string
	Token  string
}

// SendOptions describes the parameters needed to send data
type SendOptions struct {
	DocID      string
	DocType    string
	Type       string
	Recipients []*RecipientInfo
	Path       string
	DocRev     string

	fileOpts *fileOptions
}

type fileOptions struct {
	contentlength string
	mime          string
	md5           string
	queries       url.Values
	content       vfs.File
	set           bool // default value is false
}

// fillDetailsAndOpenFile will augment the SendOptions structure with the
// details regarding the file to share and open it so that it can be sent.
//
// WARNING: the file descriptor must be closed!
//
// The idea behind this function is to prevent multiple creations of a file
// descriptor, in order to limit I/O to a single opening.
// This function will set the field `set` of the SendOptions structure to `true`
// the first time it is called and thus causing later calls to immediately
// return.
func (opts *SendOptions) fillDetailsAndOpenFile(fs vfs.VFS, fileDoc *vfs.FileDoc) error {
	if opts.fileOpts != nil && opts.fileOpts.set {
		return nil
	}

	fileOpts := &fileOptions{}

	fileOpts.mime = fileDoc.Mime
	fileOpts.contentlength = strconv.FormatInt(fileDoc.ByteSize, 10)
	fileOpts.md5 = base64.StdEncoding.EncodeToString(fileDoc.MD5Sum)

	// Send references for permissions
	// TODO: only send the reference linked to the actual permission
	b, err := json.Marshal(fileDoc.ReferencedBy)
	if err != nil {
		return nil
	}
	refs := string(b[:])
	fileOpts.queries = url.Values{
		"Type":          {consts.FileType},
		"Name":          {fileDoc.DocName},
		"Executable":    {strconv.FormatBool(fileDoc.Executable)},
		"Created_at":    {fileDoc.CreatedAt.Format(time.RFC1123)},
		"Updated_at":    {fileDoc.UpdatedAt.Format(time.RFC1123)},
		"Referenced_by": []string{refs},
	}

	content, err := fs.OpenFile(fileDoc)
	if err != nil {
		return err
	}
	fileOpts.content = content
	fileOpts.set = true

	opts.fileOpts = fileOpts
	return nil
}

func (opts *SendOptions) closeFile() error {
	if opts.fileOpts != nil && opts.fileOpts.set {
		return opts.fileOpts.content.Close()
	}

	return nil
}

// SendData sends data to all the recipients
func SendData(ctx context.Context, m *jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)

	opts := &SendOptions{}
	err := m.Unmarshal(&opts)
	if err != nil {
		return err
	}

	ins, err := instance.Get(domain)
	if err != nil {
		return err
	}

	opts.Path = fmt.Sprintf("/sharings/doc/%s/%s", opts.DocType, opts.DocID)

	if opts.DocType == consts.Files {
		dirDoc, fileDoc, err := ins.VFS().DirOrFileByID(opts.DocID)
		if err != nil {
			return err
		}

		if dirDoc != nil {
			opts.Type = consts.DirType
			return SendDir(ins, opts, dirDoc)
		}

		opts.Type = consts.FileType
		return SendFile(ins, opts, fileDoc)
	}

	return SendDoc(ins, opts)
}

// DeleteDoc asks the recipients to delete the shared document which id was
// provided.
func DeleteDoc(opts *SendOptions) error {
	for _, rec := range opts.Recipients {
		doc, err := getDocAtRecipient(nil, opts.DocType, opts.DocID, rec)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to get "+
				"remote doc : ", err)
			continue
		}
		rev := doc.M["_rev"].(string)

		_, errSend := request.Req(&request.Options{
			Domain: rec.URL,
			Scheme: rec.Scheme,
			Method: http.MethodDelete,
			Path:   opts.Path,
			Headers: request.Headers{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer " + rec.Token,
			},
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

// SendDoc sends a JSON document to the recipients.
func SendDoc(ins *instance.Instance, opts *SendOptions) error {
	doc := &couchdb.JSONDoc{}
	if err := couchdb.GetDoc(ins, opts.DocType, opts.DocID, doc); err != nil {
		return err
	}

	// A new doc will be created on the recipient side
	delete(doc.M, "_id")
	delete(doc.M, "_rev")

	for _, rec := range opts.Recipients {
		errs := sendDocToRecipient(opts, rec, doc, http.MethodPost)
		if errs != nil {
			log.Error("[sharing] An error occurred while trying to send "+
				"a document to a recipient:", errs)
		}
	}

	return nil
}

// UpdateDoc updates a JSON document at each recipient.
func UpdateDoc(ins *instance.Instance, opts *SendOptions) error {
	doc := &couchdb.JSONDoc{}
	if err := couchdb.GetDoc(ins, opts.DocType, opts.DocID, doc); err != nil {
		return err
	}

	for _, rec := range opts.Recipients {
		// A doc update requires to set the doc revision from each recipient
		remoteDoc, err := getDocAtRecipient(doc, opts.DocType, opts.DocID, rec)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to get "+
				"remote doc : ", err)
			continue
		}
		// No changes: nothing to do
		if changes := docHasChanges(doc, remoteDoc); !changes {
			continue
		}
		rev := remoteDoc.M["_rev"].(string)
		doc.SetRev(rev)

		errs := sendDocToRecipient(opts, rec, doc, http.MethodPut)
		if errs != nil {
			log.Error("[sharing] An error occurred while trying to send "+
				"an update: ", err)
		}
	}

	return nil
}

func sendDocToRecipient(opts *SendOptions, rec *RecipientInfo, doc *couchdb.JSONDoc, method string) error {
	body, err := request.WriteJSON(doc.M)
	if err != nil {
		return err
	}

	// Send the document to the recipient
	// TODO : handle send failures
	_, err = request.Req(&request.Options{
		Domain: rec.URL,
		Scheme: rec.Scheme,
		Method: method,
		Path:   opts.Path,
		Headers: request.Headers{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + rec.Token,
		},
		Body:       body,
		NoResponse: true,
	})

	return err
}

// SendFile sends a binary file to the recipients
func SendFile(ins *instance.Instance, opts *SendOptions, fileDoc *vfs.FileDoc) error {
	err := opts.fillDetailsAndOpenFile(ins.VFS(), fileDoc)
	if err != nil {
		return err
	}
	defer opts.closeFile()

	for _, rec := range opts.Recipients {
		err = sendFileToRecipient(opts, rec, http.MethodPost)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to share "+
				"file "+fileDoc.DocName+": ", err)
		}
	}

	return nil
}

// SendDir sends a directory to the recipients.
func SendDir(ins *instance.Instance, opts *SendOptions, dirDoc *vfs.DirDoc) error {
	dirPath, err := dirDoc.Path(ins.VFS())
	if err != nil {
		return err
	}
	dirTags := strings.Join(dirDoc.Tags, files.TagSeparator)

	for _, recipient := range opts.Recipients {
		_, err := request.Req(&request.Options{
			Domain: recipient.URL,
			Scheme: recipient.Scheme,
			Method: http.MethodPost,
			Path:   opts.Path,
			Headers: request.Headers{
				echo.HeaderContentType:   echo.MIMEApplicationJSON,
				echo.HeaderAuthorization: "Bearer " + recipient.Token,
			},
			Queries: url.Values{
				"Tags":       {dirTags},
				"Path":       {dirPath},
				"Name":       {dirDoc.DocName},
				"Type":       {consts.DirType},
				"Created_at": {dirDoc.CreatedAt.Format(time.RFC1123)},
				"Updated_at": {dirDoc.CreatedAt.Format(time.RFC1123)},
			},
			NoResponse: true,
		})
		if err != nil {
			log.Error("[sharing] An error occurred while trying to share "+
				"the directory "+dirDoc.DocName+": ", err)
		}
	}

	return nil
}

// UpdateOrPatchFile uploads the file to the recipients if the md5sum has
// changed compared to their local version, and sends a patch if not.
func UpdateOrPatchFile(ins *instance.Instance, opts *SendOptions, fileDoc *vfs.FileDoc) error {
	md5 := base64.StdEncoding.EncodeToString(fileDoc.MD5Sum)

	// A file descriptor can be open in the for loop.
	defer opts.closeFile()

	for _, recipient := range opts.Recipients {
		// Get recipient data
		data, err := getFileOrDirMetadataAtRecipient(opts.DocID, recipient)
		if err != nil {
			log.Error("[sharing] Could not get data at "+recipient.URL+": ", err)
			continue
		}

		md5AtRec, rev := extractMD5AndRev(data, recipient)
		opts.DocRev = rev

		// The MD5 didn't change: this is a PATCH
		if md5 == md5AtRec {
			// Check the metadata did change to do the patch
			if hasChanges := fileHasChanges(fileDoc, data); !hasChanges {
				continue
			}
			patch, errp := generateDirOrFilePatch(nil, fileDoc)
			if errp != nil {
				log.Error("[sharing] Could not generate patch for file "+
					fileDoc.DocName+": ", errp)
				continue
			}
			errsp := sendPatchToRecipient(patch, opts, recipient)
			if errsp != nil {
				log.Error("[sharing] An error occurred while trying to "+
					"send patch: ", errsp)
			}

			continue
		}
		// The MD5 did change: this is a PUT
		err = opts.fillDetailsAndOpenFile(ins.VFS(), fileDoc)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to open "+
				fileDoc.DocName+": ", err)
			continue
		}
		err = sendFileToRecipient(opts, recipient, http.MethodPut)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to share an "+
				"update of file "+fileDoc.DocName+" to a recipient: ", err)
		}
	}

	return nil
}

// PatchDir updates the metadata of the corresponding directory at each
// recipient's.
func PatchDir(opts *SendOptions, dirDoc *vfs.DirDoc) error {
	patch, err := generateDirOrFilePatch(dirDoc, nil)
	if err != nil {
		return err
	}

	for _, rec := range opts.Recipients {
		rev, err := getDirOrFileRevAtRecipient(opts.DocID, rec)
		if err != nil {
			return err
		}
		opts.DocRev = rev
		err = sendPatchToRecipient(patch, opts, rec)
		if err != nil {
			log.Error("[sharing] An error occurred while trying to send "+
				"a patch: ", err)
		}
	}

	return nil
}

// DeleteDirOrFile asks the recipients to put the file or directory in the
// trash.
func DeleteDirOrFile(opts *SendOptions) error {
	for _, recipient := range opts.Recipients {
		rev, err := getDirOrFileRevAtRecipient(opts.DocID, recipient)
		if err != nil {
			log.Error("[sharing] (delete) An error occurred while trying "+
				"to get a revision at "+recipient.URL+":", err)
			continue
		}
		opts.DocRev = rev

		_, err = request.Req(&request.Options{
			Domain: recipient.URL,
			Scheme: recipient.Scheme,
			Method: http.MethodDelete,
			Path:   opts.Path,
			Headers: request.Headers{
				echo.HeaderContentType:   echo.MIMEApplicationJSON,
				echo.HeaderAuthorization: "Bearer " + recipient.Token,
			},
			Queries: url.Values{
				"rev":  {opts.DocRev},
				"Type": {opts.Type},
			},
			NoResponse: true,
		})

		if err != nil {
			log.Error("[sharing] (delete) An error occurred while sending "+
				"request to "+recipient.URL+": ", err)
		}
	}

	return nil
}

func sendFileToRecipient(opts *SendOptions, recipient *RecipientInfo, method string) error {
	if !opts.fileOpts.set {
		return errors.New("[sharing] fileOpts were not set")
	}

	if opts.DocRev != "" {
		opts.fileOpts.queries.Add("rev", opts.DocRev)
	}

	_, err := request.Req(&request.Options{
		Domain: recipient.URL,
		Scheme: recipient.Scheme,
		Method: method,
		Path:   opts.Path,
		Headers: request.Headers{
			"Content-Type":   opts.fileOpts.mime,
			"Accept":         "application/vnd.api+json",
			"Content-Length": opts.fileOpts.contentlength,
			"Content-MD5":    opts.fileOpts.md5,
			"Authorization":  "Bearer " + recipient.Token,
		},
		Queries:    opts.fileOpts.queries,
		Body:       opts.fileOpts.content,
		NoResponse: true,
	})

	return err
}

func sendPatchToRecipient(patch *jsonapi.Document, opts *SendOptions, recipient *RecipientInfo) error {
	body, err := request.WriteJSON(patch)
	if err != nil {
		return err
	}

	_, err = request.Req(&request.Options{
		Domain: recipient.URL,
		Scheme: recipient.Scheme,
		Method: http.MethodPatch,
		Path:   opts.Path,
		Headers: request.Headers{
			echo.HeaderContentType:   jsonapi.ContentType,
			echo.HeaderAuthorization: "Bearer " + recipient.Token,
		},
		Queries: url.Values{
			"rev":  {opts.DocRev},
			"Type": {opts.Type},
		},
		Body:       body,
		NoResponse: true,
	})

	return err
}

// Generates a document patch for the given document.
//
// The server expects a jsonapi.Document structure, see:
// http://jsonapi.org/format/#document-structure
// The data part of the jsonapi.Document contains an ObjectMarshalling, see:
// web/jsonapi/data.go:66
func generateDirOrFilePatch(dirDoc *vfs.DirDoc, fileDoc *vfs.FileDoc) (*jsonapi.Document, error) {
	var patch vfs.DocPatch
	var id string
	var rev string

	if dirDoc != nil {
		patch.Name = &dirDoc.DocName
		patch.DirID = &dirDoc.DirID
		patch.Tags = &dirDoc.Tags
		patch.UpdatedAt = &dirDoc.UpdatedAt
		id = dirDoc.ID()
		rev = dirDoc.Rev()
	} else {
		patch.Name = &fileDoc.DocName
		patch.DirID = &fileDoc.DirID
		patch.Tags = &fileDoc.Tags
		patch.UpdatedAt = &fileDoc.UpdatedAt
		id = fileDoc.ID()
		rev = fileDoc.Rev()
	}

	attrs, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	obj := &jsonapi.ObjectMarshalling{
		Type:       consts.Files,
		ID:         id,
		Attributes: (*json.RawMessage)(&attrs),
		Meta:       jsonapi.Meta{Rev: rev},
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	return &jsonapi.Document{Data: (*json.RawMessage)(&data)}, nil
}

// getDocAtRecipient returns the document at the given
// recipient.
func getDocAtRecipient(newDoc *couchdb.JSONDoc, doctype, docID string, recInfo *RecipientInfo) (*couchdb.JSONDoc, error) {
	path := fmt.Sprintf("/data/%s/%s", doctype, docID)

	res, err := request.Req(&request.Options{
		Domain: recInfo.URL,
		Scheme: recInfo.Scheme,
		Method: http.MethodGet,
		Path:   path,
		Headers: request.Headers{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + recInfo.Token,
		},
	})
	if err != nil {
		return nil, err
	}

	doc := &couchdb.JSONDoc{}
	if err := request.ReadJSON(res.Body, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func extractMD5AndRev(data map[string]interface{}, recipient *RecipientInfo) (md5sum, rev string) {
	attributes := data["attributes"].(map[string]interface{})
	md5sum = attributes["md5sum"].(string)
	meta := data["meta"].(map[string]interface{})
	rev = meta["rev"].(string)

	return
}

func getDirOrFileRevAtRecipient(docID string, recipient *RecipientInfo) (string, error) {
	data, err := getFileOrDirMetadataAtRecipient(docID, recipient)
	if err != nil {
		return "", err
	}

	meta := data["meta"].(map[string]interface{})
	rev := meta["rev"].(string)
	return rev, nil
}

func getFileOrDirMetadataAtRecipient(id string, recInfo *RecipientInfo) (map[string]interface{}, error) {
	path := fmt.Sprintf("/files/%s", id)

	res, err := request.Req(&request.Options{
		Domain: recInfo.URL,
		Scheme: recInfo.Scheme,
		Method: http.MethodGet,
		Path:   path,
		Headers: request.Headers{
			echo.HeaderContentType:    echo.MIMEApplicationJSON,
			echo.HeaderAcceptEncoding: echo.MIMEApplicationJSON,
			echo.HeaderAuthorization:  "Bearer " + recInfo.Token,
		},
	})
	if err != nil {
		return nil, err
	}

	doc := map[string]interface{}{}
	err = request.ReadJSON(res.Body, &doc)
	if err != nil {
		return nil, err
	}

	// What is returned by the stack has the following structure:
	// data : {
	//		attributes: {
	//			md5sum: string,
	//			…,
	//		},
	//		meta: {
	//			rev: string,
	//		},
	// }
	data := doc["data"].(map[string]interface{})
	return data, nil
}

// filehasChanges checks that the local file do have changes compared to the remote one
// This is done to prevent infinite loops after a PUT/PATCH in master-master:
// we don't propagate the update if they are similar
func fileHasChanges(newFileDoc *vfs.FileDoc, data map[string]interface{}) bool {
	//TODO: support modifications on other fields
	attributes := data["attributes"].(map[string]interface{})
	return newFileDoc.Name() != attributes["name"].(string)
}

// docHasChanges checks that the local doc do have changes compared to the remote one
// This is done to prevent infinite loops after a PUT/PATCH in master-master:
// we don't mitigate the update if they are similar.
func docHasChanges(newDoc *couchdb.JSONDoc, doc *couchdb.JSONDoc) bool {

	// Compare the incoming doc and the existing one without the _id and _rev
	newID := newDoc.M["_id"].(string)
	newRev := newDoc.M["_rev"].(string)
	rev := doc.M["_rev"].(string)
	delete(newDoc.M, "_id")
	delete(newDoc.M, "_rev")
	delete(doc.M, "_id")
	delete(doc.M, "_rev")

	isEqual := reflect.DeepEqual(newDoc.M, doc.M)

	newDoc.M["_id"] = newID
	newDoc.M["_rev"] = newRev
	doc.M["_rev"] = rev

	return !isEqual
}
