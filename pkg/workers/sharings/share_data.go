package sharings

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/jsonapi"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/labstack/echo"
)

func init() {
	jobs.AddWorker("sharedata", &jobs.WorkerConfig{
		Concurrency: 1, // no concurency, to make sure directory hierarchy order is respected
		WorkerFunc:  SendData,
	})
}

// SendOptions describes the parameters needed to send data
type SendOptions struct {
	DocID      string
	DocType    string
	SharingID  string
	Type       string
	Recipients []*sharings.RecipientInfo
	Path       string
	DocRev     string

	Selector   string
	Values     []string
	sharedRefs []couchdb.DocReference

	fileOpts *fileOptions
	dirOpts  *dirOptions
}

type fileOptions struct {
	contentlength string
	mime          string
	md5           string
	queries       url.Values
	set           bool // default value is false
}

type dirOptions struct {
	tags  string
	refs  string
	dirID string
}

var (
	// ErrBadFileFormat is used when the given file is not well structured
	ErrBadFileFormat = errors.New("Bad file format")
	//ErrRemoteDocDoesNotExist is used when the remote doc does not exist
	ErrRemoteDocDoesNotExist = errors.New("Remote doc does not exist")
	// ErrBadPermission is used when a given permission is not valid
	ErrBadPermission = errors.New("Invalid permission format")
	// ErrForbidden is used when the recipient returned a 403 error
	ErrForbidden = errors.New("Forbidden")
)

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
	var refs, dirID string
	if opts.Selector == consts.SelectorReferencedBy {
		sharedRefs := opts.getSharedReferences()
		b, err := json.Marshal(sharedRefs)
		if err != nil {
			return err
		}
		refs = string(b)
	}

	// Specify the dirID if it part of a directory sharing
	if isDirSharing(fs, opts) {
		dirID = fileDoc.DirID
	}

	fileOpts.queries = url.Values{
		consts.QueryParamSharingID:    {opts.SharingID},
		consts.QueryParamType:         {consts.FileType},
		consts.QueryParamName:         {fileDoc.DocName},
		consts.QueryParamExecutable:   {strconv.FormatBool(fileDoc.Executable)},
		consts.QueryParamCreatedAt:    {fileDoc.CreatedAt.Format(time.RFC1123)},
		consts.QueryParamUpdatedAt:    {fileDoc.UpdatedAt.Format(time.RFC1123)},
		consts.QueryParamReferencedBy: {refs},
		consts.QueryParamDirID:        {dirID},
	}

	fileOpts.set = true
	opts.fileOpts = fileOpts
	return nil
}

// If the selector is "referenced_by" then the values are of the form
// "doctype/id". To be able to use them we first need to parse them.
func (opts *SendOptions) getSharedReferences() []couchdb.DocReference {
	if opts.sharedRefs == nil && opts.Selector == consts.SelectorReferencedBy {
		opts.sharedRefs = []couchdb.DocReference{}
		for _, ref := range opts.Values {
			parts := strings.Split(ref, permissions.RefSep)
			if len(parts) != 2 {
				continue
			}

			opts.sharedRefs = append(opts.sharedRefs, couchdb.DocReference{
				Type: parts[0],
				ID:   parts[1],
			})
		}
	}

	return opts.sharedRefs
}

// This function extracts only the relevant references: those that concern the
// sharing.
//
// `sharedRefs` is the set of shared references. The result is thus a subset of
// it or all.
func (opts *SendOptions) extractRelevantReferences(refs []couchdb.DocReference) []couchdb.DocReference {
	var res []couchdb.DocReference

	sharedRefs := opts.getSharedReferences()

	for i, ref := range refs {
		for _, sharedRef := range sharedRefs {
			if ref.ID == sharedRef.ID {
				res = append(res, refs[i])
				break
			}
		}
	}

	return res
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
			ins.Logger().Debugf("[sharings] share_data: Sending directory: %v",
				dirDoc)
			return SendDir(ins, opts, dirDoc)
		}
		opts.Type = consts.FileType
		ins.Logger().Debugf("[sharings] share_data: Sending file: %v", fileDoc)
		return SendFile(ins, opts, fileDoc)
	}

	ins.Logger().Debugf("[sharings] share_data: Sending %s: %s", opts.DocType,
		opts.DocID)
	return SendDoc(ins, opts)
}

// DeleteDoc asks the recipients to delete the shared document which id was
// provided.
func DeleteDoc(ins *instance.Instance, opts *SendOptions) error {
	var errFinal error

	for _, recipient := range opts.Recipients {
		doc, err := getDocAtRecipient(ins, nil, opts, recipient)
		if err != nil {
			errFinal = multierror.Append(errFinal,
				fmt.Errorf("Error while trying to get remote doc : %s",
					err.Error()))
			continue
		}
		rev := doc.M["_rev"].(string)

		reqOpts := &request.Options{
			Domain: recipient.URL,
			Scheme: recipient.Scheme,
			Method: http.MethodDelete,
			Path:   opts.Path,
			Headers: request.Headers{
				"Content-Type":  "application/json",
				"Accept":        "application/json",
				"Authorization": "Bearer " + recipient.AccessToken.AccessToken,
			},
			Queries: url.Values{
				consts.QueryParamSharingID: {opts.SharingID},
				consts.QueryParamRev:       {rev},
			},
			NoResponse: true,
		}
		_, errSend := request.Req(reqOpts)

		if errSend != nil {
			if sharings.AuthError(errSend) {
				_, errSend = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recipient, reqOpts)
			}
			if errSend != nil {
				errFinal = multierror.Append(errFinal, fmt.Errorf("Error while trying to share data : %s", errSend.Error()))
			}
		}
	}

	return errFinal
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
		errs := sendDocToRecipient(ins, opts, rec, doc, http.MethodPost)
		if errs != nil {
			ins.Logger().Error("[sharing] An error occurred while trying to"+
				" send a document to a recipient: ", errs)
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
		remoteDoc, err := getDocAtRecipient(ins, doc, opts, rec)
		if err != nil {
			ins.Logger().Error("[sharings] An error occurred while trying to "+
				"get remote doc : ", err)
			continue
		}
		// No changes: nothing to do
		if !docHasChanges(doc, remoteDoc) {
			continue
		}
		rev := remoteDoc.M["_rev"].(string)
		doc.SetRev(rev)

		errs := sendDocToRecipient(ins, opts, rec, doc, http.MethodPut)
		if errs != nil {
			ins.Logger().Error("[sharings] An error occurred while trying to "+
				"send an update: ", err)
		}
	}

	return nil
}

func sendDocToRecipient(ins *instance.Instance, opts *SendOptions, rec *sharings.RecipientInfo, doc *couchdb.JSONDoc, method string) error {
	body, err := request.WriteJSON(doc.M)
	if err != nil {
		return err
	}
	// Send the document to the recipient
	// TODO : handle send failures
	reqOpts := &request.Options{
		Domain: rec.URL,
		Scheme: rec.Scheme,
		Method: method,
		Path:   opts.Path,
		Headers: request.Headers{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + rec.AccessToken.AccessToken,
		},
		Queries: url.Values{
			consts.QueryParamSharingID: {opts.SharingID},
		},
		Body:       body,
		NoResponse: true,
	}
	_, err = request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			body, berr := request.WriteJSON(doc.M)
			if berr != nil {
				return berr
			}
			reqOpts.Body = body
			_, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, rec, reqOpts)
		}
	}

	return err
}

// SendFile sends a binary file to the recipients.
//
// To prevent any useless sending we first make a HEAD request on the file. If
// we have a 404 or a 403 error then we can safely assume that the sending is
// legitimate.
//
func SendFile(ins *instance.Instance, opts *SendOptions, fileDoc *vfs.FileDoc) error {
	err := opts.fillDetailsAndOpenFile(ins.VFS(), fileDoc)
	if err != nil {
		return err
	}

	for _, recipient := range opts.Recipients {
		// Check remote existence of the file
		err = headDirOrFileMetadataAtRecipient(ins, opts.SharingID, opts.DocID,
			consts.FileType, recipient)

		if err == ErrRemoteDocDoesNotExist || err == ErrForbidden {
			err = sendFileToRecipient(ins, fileDoc, opts, recipient, http.MethodPost)
			if err != nil {
				ins.Logger().Errorf("[sharings] An error occurred while "+
					"trying to share file %v: %v", fileDoc.ID(), err)
			}

		} else {
			if err == nil {
				ins.Logger().Debugf("[sharings] Aborting: recipient already "+
					"has the file: %s", fileDoc.ID())
			} else {
				ins.Logger().Debugf("[sharings] Aborting: %v", err)
			}
		}
	}

	return nil
}

// SendDir sends a directory to the recipients.
//
func SendDir(ins *instance.Instance, opts *SendOptions, dirDoc *vfs.DirDoc) error {
	dirOpts := &dirOptions{}

	dirTags := strings.Join(dirDoc.Tags, files.TagSeparator)
	dirOpts.tags = dirTags

	var refs string
	if opts.Selector == consts.SelectorReferencedBy {
		sharedRefs := opts.getSharedReferences()
		b, errm := json.Marshal(sharedRefs)
		if errm != nil {
			return errm
		}
		refs = string(b)
	}
	dirOpts.refs = refs

	// Specify the dirID only if the directory is not the sharing container
	dirID := dirDoc.DirID
	if dirIsSharedContainer(opts, dirDoc.ID()) {
		dirID = ""
	}
	dirOpts.dirID = dirID
	opts.dirOpts = dirOpts

	for _, recipient := range opts.Recipients {
		// Check remote existence of the directory
		err := headDirOrFileMetadataAtRecipient(ins, opts.SharingID, opts.DocID,
			consts.DirType, recipient)
		if err == ErrRemoteDocDoesNotExist || err == ErrForbidden {
			err = sendDirToRecipient(ins, dirDoc, opts, recipient)
			if err != nil {
				ins.Logger().Errorf("[sharings] An error occurred while "+
					"trying to share directory %v: %v", dirDoc.ID(), err)
			}

		} else {
			if err == nil {
				ins.Logger().Debugf("[sharings] Aborting: recipient already "+
					"has the directory: %s", dirDoc.ID())
			} else {
				ins.Logger().Debugf("[sharings] Aborting: %v", err)
			}
		}
	}

	return nil
}

// UpdateOrPatchFile updates the file at the recipients.
//
// Depending on the type of update several actions are possible:
// 1. The actual content of the file was modified so we need to upload the new
//    version to the recipients.
//        -> we send the file.
// 2. The event is dectected as a modification but the recipient does not have
//    it (404) or does not let us access it (403 - the file is already shared
//    in another sharing).
//        -> we send the file.
// 3. The name of the file has changed.
//        -> we change the metadata.
// 4. The references of the file have changed.
//        -> we update the references.
//
func UpdateOrPatchFile(ins *instance.Instance, opts *SendOptions, fileDoc *vfs.FileDoc, sendToSharer bool) error {
	md5 := base64.StdEncoding.EncodeToString(fileDoc.MD5Sum)

	for _, recipient := range opts.Recipients {
		_, remoteFileDoc, err := getDirOrFileMetadataAtRecipient(ins, opts,
			recipient)

		if err != nil {
			if err == ErrRemoteDocDoesNotExist || err == ErrForbidden {
				errf := opts.fillDetailsAndOpenFile(ins.VFS(), fileDoc)
				if errf != nil {
					return err
				}
				errf = sendFileToRecipient(ins, fileDoc, opts, recipient, http.MethodPost)
				if errf != nil {
					ins.Logger().Error("[sharings] An error occurred while "+
						"trying to send file: ", errf)
				}

			} else {
				ins.Logger().Errorf("[sharings] Could not get data at %v: %v",
					recipient.URL, err)
			}

			continue
		}

		md5AtRec := base64.StdEncoding.EncodeToString(remoteFileDoc.MD5Sum)
		opts.DocRev = remoteFileDoc.Rev()

		// The MD5 didn't change: this is a PATCH or a reference update.
		if md5 == md5AtRec {
			// Check the metadata did change to do the patch
			if !fileHasChanges(ins.VFS(), opts, fileDoc, remoteFileDoc) {
				// Special case to deal with ReferencedBy fields
				if opts.Selector == consts.SelectorReferencedBy {
					refs := findNewRefs(opts, fileDoc, remoteFileDoc)
					if refs != nil {
						erru := updateReferencesAtRecipient(ins, http.MethodPost,
							refs, opts, recipient, sendToSharer)
						if erru != nil {
							ins.Logger().Error("[sharings] An error occurred "+
								" while trying to update references: ", erru)
						}
					}
				}
				continue
			}
			patch, errp := generateDirOrFilePatch(ins.VFS(), opts, nil, fileDoc)
			if errp != nil {
				ins.Logger().Errorf("[sharings] Could not generate patch for "+
					"file %v: %v", fileDoc.DocName, errp)
				continue
			}
			errsp := sendPatchToRecipient(ins, patch, opts, recipient, fileDoc.DirID)
			if errsp != nil {
				ins.Logger().Error("[sharings] An error occurred while trying "+
					"to send patch: ", errsp)
			}
		} else {
			// The MD5 did change: this is a PUT
			err = opts.fillDetailsAndOpenFile(ins.VFS(), fileDoc)
			if err != nil {
				ins.Logger().Errorf("[sharings] An error occurred while trying "+
					"to open %v: %v", fileDoc.DocName, err)
				continue
			}
			err = sendFileToRecipient(ins, fileDoc, opts, recipient, http.MethodPut)
			if err != nil {
				ins.Logger().Errorf("[sharings] An error occurred while trying to "+
					"share an update of file %v to a recipient: %v",
					fileDoc.DocName, err)
			}
		}

	}

	return nil
}

// PatchDir updates the metadata of the corresponding directory at each
// recipient's.
func PatchDir(ins *instance.Instance, opts *SendOptions, dirDoc *vfs.DirDoc) error {
	var errFinal error

	patch, err := generateDirOrFilePatch(ins.VFS(), opts, dirDoc, nil)
	if err != nil {
		return err
	}

	for _, rec := range opts.Recipients {
		remoteDirDoc, _, err := getDirOrFileMetadataAtRecipient(ins, opts, rec)
		if err != nil {
			return err
		}
		opts.DocRev = remoteDirDoc.Rev()

		// Share only if directories have changes
		if dirHasChanges(ins.VFS(), opts, dirDoc, remoteDirDoc) {
			err = sendPatchToRecipient(ins, patch, opts, rec, dirDoc.DirID)
			if err != nil {
				errFinal = multierror.Append(errFinal,
					fmt.Errorf("Error while trying to send a patch: %s",
						err.Error()))
			}
		}
	}

	return errFinal
}

// RemoveDirOrFileFromSharing tells the recipient to remove the file or
// directory from the specified sharing.
//
// If we are asking the sharer to remove the file from the sharing then the
// recipient has to check if that file is still shared in another sharing. If
// not it must be trashed.
//
// As of now since we only support sharings through ids or "referenced_by"
// selector the only event that could lead to calling this function would be a
// set of "referenced_by" not applying anymore.
//
// TODO Handle sharing of directories
func RemoveDirOrFileFromSharing(ins *instance.Instance, opts *SendOptions, sendToSharer bool) error {
	sharedRefs := opts.getSharedReferences()

	if sendToSharer {
		err := sharings.RemoveDocumentIfNotShared(ins, opts.DocType, opts.DocID)
		if err != nil {
			return err
		}
	}

	for _, recipient := range opts.Recipients {
		errs := updateReferencesAtRecipient(ins, http.MethodDelete, sharedRefs,
			opts, recipient, sendToSharer)
		if errs != nil {
			ins.Logger().Debugf("[sharings] Could not update reference at "+
				"recipient: %v", errs)
		}
	}

	return nil
}

// DeleteDirOrFile asks the recipients to put the file or directory in the
// trash, if hardDelete is false
func DeleteDirOrFile(ins *instance.Instance, opts *SendOptions, hardDelete bool) error {
	var errFinal error
	for _, recipient := range opts.Recipients {
		rev, err := getDirOrFileRevAtRecipient(ins, opts, recipient)
		if err != nil {
			errFinal = multierror.Append(errFinal,
				fmt.Errorf("Error while trying to get a revision at %v: %v",
					recipient.URL, err))
			continue
		}
		opts.DocRev = rev

		reqOpts := &request.Options{
			Domain: recipient.URL,
			Scheme: recipient.Scheme,
			Method: http.MethodDelete,
			Path:   opts.Path,
			Headers: request.Headers{
				echo.HeaderContentType:   echo.MIMEApplicationJSON,
				echo.HeaderAuthorization: "Bearer " + recipient.AccessToken.AccessToken,
			},
			Queries: url.Values{
				consts.QueryParamSharingID: {opts.SharingID},
				consts.QueryParamRev:       {opts.DocRev},
				consts.QueryParamType:      {opts.Type},
				"hard_delete":              {strconv.FormatBool(hardDelete)},
			},
			NoResponse: true,
		}
		_, err = request.Req(reqOpts)
		if err != nil {
			if sharings.AuthError(err) {
				_, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recipient, reqOpts)
			}
			if err != nil {
				errFinal = multierror.Append(errFinal,
					fmt.Errorf("Error while sending request to %v: %v", recipient.URL, err))
			}
		}
	}

	return nil
}

// Send the file to the recipient.
//
// Two scenarii are possible:
// 1. `opts.DocRev` is empty: the recipient should not have the file in his
//    Cozy.
// 2. `opts.DocRev` is NOT empty: the recipient already has the file and the
//    sharer is updating it.
func sendFileToRecipient(ins *instance.Instance, fileDoc *vfs.FileDoc, opts *SendOptions, recipient *sharings.RecipientInfo, method string) error {
	if !opts.fileOpts.set {
		return errors.New("[sharings] fileOpts were not set")
	}

	if opts.DocRev != "" {
		opts.fileOpts.queries.Add("rev", opts.DocRev)
	}

	content, err := ins.VFS().OpenFile(fileDoc)
	if err != nil {
		return err
	}
	defer content.Close()

	reqOpts := &request.Options{
		Domain: recipient.URL,
		Scheme: recipient.Scheme,
		Method: method,
		Path:   opts.Path,
		Headers: request.Headers{
			"Content-Type":   opts.fileOpts.mime,
			"Accept":         "application/vnd.api+json",
			"Content-Length": opts.fileOpts.contentlength,
			"Content-MD5":    opts.fileOpts.md5,
			"Authorization":  "Bearer " + recipient.AccessToken.AccessToken,
		},
		Queries:    opts.fileOpts.queries,
		Body:       content,
		NoResponse: true,
	}
	_, err = request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			content, erro := ins.VFS().OpenFile(fileDoc)
			if erro != nil {
				return erro
			}
			reqOpts.Body = content
			_, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recipient, reqOpts)
		}
	}
	return err
}

// Send the file to the recipient.
func sendDirToRecipient(ins *instance.Instance, dirDoc *vfs.DirDoc, opts *SendOptions, recipient *sharings.RecipientInfo) error {
	reqOpts := &request.Options{
		Domain: recipient.URL,
		Scheme: recipient.Scheme,
		Method: http.MethodPost,
		Path:   opts.Path,
		Headers: request.Headers{
			echo.HeaderContentType:   echo.MIMEApplicationJSON,
			echo.HeaderAuthorization: "Bearer " + recipient.AccessToken.AccessToken,
		},
		Queries: url.Values{
			consts.QueryParamSharingID: {opts.SharingID},
			consts.QueryParamTags:      {opts.dirOpts.tags},
			consts.QueryParamName:      {dirDoc.DocName},
			consts.QueryParamType:      {consts.DirType},
			consts.QueryParamCreatedAt: {
				dirDoc.CreatedAt.Format(time.RFC1123)},
			consts.QueryParamUpdatedAt: {
				dirDoc.CreatedAt.Format(time.RFC1123)},
			consts.QueryParamReferencedBy: {opts.dirOpts.refs},
			consts.QueryParamDirID:        {opts.dirOpts.dirID},
		},
		NoResponse: true,
	}
	_, err := request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			_, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recipient, reqOpts)
		}
		if err != nil {
			ins.Logger().Errorf("[sharing] An error occurred while trying to "+
				"share the directory %v: %v", dirDoc.DocName, err)
		}
	}
	return err
}

func sendPatchToRecipient(ins *instance.Instance, patch *jsonapi.Document, opts *SendOptions, recipient *sharings.RecipientInfo, dirID string) error {
	body, err := request.WriteJSON(patch)
	if err != nil {
		return err
	}

	reqOpts := &request.Options{
		Domain: recipient.URL,
		Scheme: recipient.Scheme,
		Method: http.MethodPatch,
		Path:   opts.Path,
		Headers: request.Headers{
			echo.HeaderContentType:   jsonapi.ContentType,
			echo.HeaderAuthorization: "Bearer " + recipient.AccessToken.AccessToken,
		},
		Queries: url.Values{
			consts.QueryParamSharingID: {opts.SharingID},
			consts.QueryParamRev:       {opts.DocRev},
			consts.QueryParamType:      {opts.Type},
		},
		Body:       body,
		NoResponse: true,
	}
	_, err = request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			body, errw := request.WriteJSON(patch)
			if errw != nil {
				return errw
			}
			reqOpts.Body = body
			_, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recipient, reqOpts)
		}
	}
	return err
}

// Depending on the `method` given this function does two things:
// 1. If it's "POST" it calls the regular routes for adding references to files.
// 2. If it's "DELETE" it calls the sharing handler because, in addition to
//    removing the references, we need to see if the file is still shared and if
//    not we need to trash it.
func updateReferencesAtRecipient(ins *instance.Instance, method string, refs []couchdb.DocReference, opts *SendOptions, recipient *sharings.RecipientInfo, sendToSharer bool) error {
	data, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	doc := jsonapi.Document{
		Data: (*json.RawMessage)(&data),
	}
	body, err := request.WriteJSON(doc)
	if err != nil {
		return err
	}

	var path string
	if method == http.MethodPost {
		path = fmt.Sprintf("/files/%s/relationships/referenced_by", opts.DocID)
	} else {
		path = fmt.Sprintf("/sharings/files/%s/referenced_by", opts.DocID)
	}

	values := url.Values{
		consts.QueryParamSharingID: {opts.SharingID},
		consts.QueryParamSharer:    {strconv.FormatBool(sendToSharer)},
	}

	reqOpts := &request.Options{
		Domain:  recipient.URL,
		Scheme:  recipient.Scheme,
		Method:  method,
		Path:    path,
		Queries: values,
		Headers: request.Headers{
			echo.HeaderContentType: jsonapi.ContentType,
			echo.HeaderAuthorization: "Bearer " +
				recipient.AccessToken.AccessToken,
		},
		Body:       body,
		NoResponse: true,
	}
	_, err = request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			body, errw := request.WriteJSON(doc)
			if errw != nil {
				return errw
			}
			reqOpts.Body = body
			_, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recipient, reqOpts)
		}
	}
	return err
}

// NOTE: the root folder cannot be shared yet
func isShared(fs vfs.VFS, id string, acceptedIDs []string) bool {
	if id == consts.RootDirID {
		return false
	}

	for _, acceptedID := range acceptedIDs {
		if id == acceptedID {
			return true
		}

		for id != consts.RootDirID {
			dirDoc, fileDoc, err := fs.DirOrFileByID(id)
			if err != nil {
				break
			}
			if dirDoc != nil {
				if dirDoc.DirID == acceptedID {
					return true
				}
				id = dirDoc.DirID
			}
			if fileDoc != nil {
				if fileDoc.DirID == acceptedID {
					return true
				}
				id = fileDoc.DirID
			}
		}
	}

	return false
}

// Generates a document patch for the given document.
//
// The server expects a jsonapi.Document structure, see:
// http://jsonapi.org/format/#document-structure
// The data part of the jsonapi.Document contains an ObjectMarshalling, see:
// web/jsonapi/data.go:66
func generateDirOrFilePatch(fs vfs.VFS, opts *SendOptions, dirDoc *vfs.DirDoc, fileDoc *vfs.FileDoc) (*jsonapi.Document, error) {
	var patch vfs.DocPatch
	var id, rev, dirID string

	if dirDoc != nil {
		dirID = dirDoc.DirID
		if opts.Selector == "" {
			// Specify the dirID only if the directory is not the sharing container
			if dirIsSharedContainer(opts, dirDoc.ID()) {
				dirID = ""
			}
		}
		patch.DirID = &dirID
		patch.Name = &dirDoc.DocName
		patch.Tags = &dirDoc.Tags
		patch.UpdatedAt = &dirDoc.UpdatedAt
		id = dirDoc.ID()
		rev = dirDoc.Rev()
	} else {
		if isDirSharing(fs, opts) {
			// The dirID is only needed for directory sharing
			dirID = fileDoc.DirID
		}
		patch.Name = &fileDoc.DocName
		patch.DirID = &dirID
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

// getDocAtRecipient returns the document at the given recipient.
func getDocAtRecipient(ins *instance.Instance, newDoc *couchdb.JSONDoc, opts *SendOptions, recInfo *sharings.RecipientInfo) (*couchdb.JSONDoc, error) {
	path := fmt.Sprintf("/data/%s/%s", opts.DocType, opts.DocID)

	reqOpts := &request.Options{
		Domain: recInfo.URL,
		Scheme: recInfo.Scheme,
		Method: http.MethodGet,
		Path:   path,
		Headers: request.Headers{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + recInfo.AccessToken.AccessToken,
		},
	}
	var res *http.Response
	var err error
	res, err = request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			res, err = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recInfo, reqOpts)
			if err != nil {
				return nil, err
			}
		}
	}
	doc := &couchdb.JSONDoc{}
	if err := request.ReadJSON(res.Body, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func getDirOrFileRevAtRecipient(ins *instance.Instance, opts *SendOptions, recipient *sharings.RecipientInfo) (string, error) {
	var rev string
	dirDoc, fileDoc, err := getDirOrFileMetadataAtRecipient(ins, opts, recipient)
	if err != nil {
		return "", err
	}
	if dirDoc != nil {
		rev = dirDoc.Rev()
	} else if fileDoc != nil {
		rev = fileDoc.Rev()
	}

	return rev, nil
}

func getDirOrFileMetadataAtRecipient(ins *instance.Instance, opts *SendOptions, recInfo *sharings.RecipientInfo) (*vfs.DirDoc, *vfs.FileDoc, error) {
	path := fmt.Sprintf("/files/%s", opts.DocID)

	reqOpts := &request.Options{
		Domain: recInfo.URL,
		Scheme: recInfo.Scheme,
		Method: http.MethodGet,
		Path:   path,
		Headers: request.Headers{
			echo.HeaderContentType:    echo.MIMEApplicationJSON,
			echo.HeaderAcceptEncoding: echo.MIMEApplicationJSON,
			echo.HeaderAuthorization:  "Bearer " + recInfo.AccessToken.AccessToken,
		},
	}

	var res *http.Response
	var rerr error

	res, err := request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			res, rerr = sharings.RefreshTokenAndRetry(ins, opts.SharingID, recInfo,
				reqOpts)
			if rerr != nil {
				return nil, nil, parseError(rerr)
			}
		} else {
			return nil, nil, parseError(err)
		}
	}

	dirOrFileDoc, err := bindDirOrFile(res.Body)
	if err != nil {
		return nil, nil, err
	}
	if dirOrFileDoc == nil {
		return nil, nil, ErrBadFileFormat
	}
	dirDoc, fileDoc := dirOrFileDoc.Refine()
	return dirDoc, fileDoc, nil
}

func headDirOrFileMetadataAtRecipient(ins *instance.Instance, sharingID, id, headType string, recInfo *sharings.RecipientInfo) error {
	path := fmt.Sprintf("/files/%s", id)
	queries := url.Values{
		"Type": {headType},
	}
	reqOpts := &request.Options{
		Domain: recInfo.URL,
		Scheme: recInfo.Scheme,
		Method: http.MethodHead,
		Path:   path,
		Headers: request.Headers{
			echo.HeaderContentType:   echo.MIMEApplicationJSON,
			echo.HeaderAuthorization: "Bearer " + recInfo.AccessToken.AccessToken,
		},
		Queries: queries,
	}

	_, err := request.Req(reqOpts)
	if err != nil {
		if sharings.AuthError(err) {
			_, err = sharings.RefreshTokenAndRetry(ins, sharingID, recInfo, reqOpts)
		}

		return parseError(err)
	}

	return nil
}

func parseError(err error) error {
	errReq, ok := err.(*request.Error)
	if !ok {
		return err
	}

	if errReq.Status == strconv.Itoa(http.StatusNotFound) ||
		errReq.Title == "Not Found" {
		return ErrRemoteDocDoesNotExist
	}
	if errReq.Status == strconv.Itoa(http.StatusForbidden) ||
		errReq.Title == "Forbidden" {
		return ErrForbidden
	}

	return errors.New(errReq.Error())
}

// filehasChanges checks that the local file do have changes compared to the
// remote one.
// This is done to prevent infinite loops after a PUT/PATCH in master-master:
// we don't propagate the update if they are similar.
func fileHasChanges(fs vfs.VFS, opts *SendOptions, newFileDoc, remoteFileDoc *vfs.FileDoc) bool {
	if newFileDoc.Name() != remoteFileDoc.Name() {
		return true
	}
	if !reflect.DeepEqual(newFileDoc.Tags, remoteFileDoc.Tags) {
		return true
	}
	// Handle dirID change for directory sharing
	if isDirSharing(fs, opts) {
		if newFileDoc.DirID != remoteFileDoc.DirID {
			return true
		}
	}

	return false
}

// dirHasChanges checks that the local directory do have changes compared to the
// remote one.
// This is done to prevent infinite loops after a PUT/PATCH in master-master:
// we don't propagate the update if they are similar.
func dirHasChanges(fs vfs.VFS, opts *SendOptions, newDirDoc, remoteDirDoc *vfs.DirDoc) bool {
	if newDirDoc.Name() != remoteDirDoc.Name() {
		return true
	}
	// Handle dirID change for directory sharing
	if isDirSharing(fs, opts) {
		if !dirIsSharedContainer(opts, newDirDoc.ID()) {
			if newDirDoc.DirID != remoteDirDoc.DirID {
				return true
			}
		}
	}
	return false
}

// docHasChanges checks that the local doc do have changes compared to the
// remote one.
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

// findNewRefs returns the references the remote is missing or nil if the remote
// is up to date with the local version of the file.
//
// This function does not deal with removing references or updating the local
// (i.e. if the remote has more references).
func findNewRefs(opts *SendOptions, fileDoc, remoteFileDoc *vfs.FileDoc) []couchdb.DocReference {
	refs := opts.extractRelevantReferences(fileDoc.ReferencedBy)
	remoteRefs := opts.extractRelevantReferences(remoteFileDoc.ReferencedBy)

	if len(refs) > len(remoteRefs) {
		return findMissingRefs(refs, remoteRefs)
	}

	return nil
}

func findMissingRefs(lref, rref []couchdb.DocReference) []couchdb.DocReference {
	var refs []couchdb.DocReference
	for _, lr := range lref {
		hasRef := false
		for _, rr := range rref {
			if rr.ID == lr.ID && rr.Type == lr.Type {
				hasRef = true
			}
		}
		if !hasRef {
			refs = append(refs, lr)
		}
	}
	return refs
}

func bindDirOrFile(body io.Reader) (*vfs.DirOrFileDoc, error) {
	decoder := json.NewDecoder(body)
	var doc *jsonapi.Document
	var dirOrFileDoc *vfs.DirOrFileDoc

	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.Data == nil {
		return nil, jsonapi.BadJSON()
	}
	var obj *jsonapi.ObjectMarshalling
	if err := json.Unmarshal(*doc.Data, &obj); err != nil {
		return nil, err
	}
	if obj.Attributes != nil {
		if err := json.Unmarshal(*obj.Attributes, &dirOrFileDoc); err != nil {
			return nil, err
		}
	}
	if rel, ok := obj.GetRelationship(consts.SelectorReferencedBy); ok {
		if res, ok := rel.Data.([]interface{}); ok {
			var refs []couchdb.DocReference
			for _, r := range res {
				if m, ok := r.(map[string]interface{}); ok {
					idd, _ := m["id"].(string)
					typ, _ := m["type"].(string)
					ref := couchdb.DocReference{ID: idd, Type: typ}
					refs = append(refs, ref)
				}
			}
			dirOrFileDoc.ReferencedBy = refs
		}
	}
	dirOrFileDoc.SetID(obj.ID)
	dirOrFileDoc.SetRev(obj.Meta.Rev)

	return dirOrFileDoc, nil
}

// dirIsSharedContainer returns true if the given dirID is the sharing container
func dirIsSharedContainer(opts *SendOptions, dirID string) bool {
	for _, val := range opts.Values {
		if val == dirID {
			return true
		}
	}
	return false
}

// isDirSharing returns true if it is a directory sharing
func isDirSharing(fs vfs.VFS, opts *SendOptions) bool {
	if opts.Selector == "" && opts.DocType == consts.Files {
		for _, val := range opts.Values {
			dirDoc, _, err := fs.DirOrFileByID(val)
			if err != nil {
				return false
			}
			if dirDoc != nil {
				return true
			}
		}
	}
	return false
}
