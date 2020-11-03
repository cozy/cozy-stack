package move

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Cursor can be used to know which files must be included in a part.
type Cursor struct {
	Number  int
	Doctype string
	ID      string
}

// String returns a string representation of the cursor.
func (c Cursor) String() string {
	return fmt.Sprintf("%s/%s", c.Doctype, c.ID)
}

// ParseCursor checks that the given cursor is part of our pre-defined list of
// cursors.
func ParseCursor(exportDoc *ExportDoc, cursorStr string) (Cursor, error) {
	if cursorStr == "" {
		return Cursor{0, consts.Files, ""}, nil
	}
	for i, c := range exportDoc.PartsCursors {
		if c == cursorStr {
			return parseCursor(i+1, cursorStr)
		}
	}
	return Cursor{}, ErrExportInvalidCursor
}

func parseCursor(number int, cursorStr string) (Cursor, error) {
	parts := strings.SplitN(cursorStr, "/", 2)
	if len(parts) != 2 {
		return Cursor{}, ErrExportInvalidCursor
	}
	return Cursor{number, parts[0], parts[1]}, nil
}

func splitFiles(partsSize, remaining int64, sizesByID map[string]int64, doctype string) ([]string, int64) {
	ids := make([]string, 0, len(sizesByID))
	for id := range sizesByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	cursors := make([]string, 0)
	for _, id := range ids {
		size := sizesByID[id]
		if size > remaining && remaining != partsSize {
			remaining = partsSize
			cursor := Cursor{0, doctype, id}.String()
			cursors = append(cursors, cursor)
		}
		remaining -= size
	}

	return cursors, remaining
}

func listFilesFromCursor(inst *instance.Instance, exportDoc *ExportDoc, start Cursor) ([]*vfs.FileDoc, error) {
	if start.Doctype != consts.Files {
		return []*vfs.FileDoc{}, nil
	}

	var end Cursor
	if start.Number < len(exportDoc.PartsCursors) {
		c, err := parseCursor(start.Number+1, exportDoc.PartsCursors[start.Number])
		if err != nil {
			return nil, err
		}
		end = c
	}
	if end.Doctype != consts.Files {
		end = Cursor{len(exportDoc.PartsCursors), consts.Files, couchdb.MaxString}
	}

	var files []*vfs.FileDoc
	req := couchdb.AllDocsRequest{
		StartKeyDocID: start.ID,
		EndKeyDocID:   end.ID,
		Limit:         1000,
	}
	for {
		var results []*vfs.FileDoc
		if err := couchdb.GetAllDocs(inst, consts.Files, &req, &results); err != nil {
			return nil, err
		}
		if len(results) == 0 {
			break
		}
		for _, res := range results {
			if res.DocID == end.ID {
				return files, nil
			}
			if res.Type == consts.FileType { // Exclude the directories
				files = append(files, res)
			}
		}
		req.StartKeyDocID = results[len(results)-1].DocID
		req.Skip = 1 // Do not fetch again the last file from this page
	}

	return files, nil
}

func listVersionsFromCursor(inst *instance.Instance, exportDoc *ExportDoc, start Cursor) ([]*vfs.Version, error) {
	var end Cursor
	if start.Number < len(exportDoc.PartsCursors) {
		c, err := parseCursor(start.Number+1, exportDoc.PartsCursors[start.Number])
		if err != nil {
			return nil, err
		}
		end = c
	}
	if end.Doctype == consts.Files {
		return []*vfs.Version{}, nil
	} else if end.Doctype == "" {
		end = Cursor{len(exportDoc.PartsCursors), consts.FilesVersions, couchdb.MaxString}
	}

	if start.Doctype != consts.FilesVersions {
		start = Cursor{start.Number, consts.FilesVersions, ""}
	}

	var versions []*vfs.Version
	req := couchdb.AllDocsRequest{
		StartKeyDocID: start.ID,
		EndKeyDocID:   end.ID,
		Limit:         1000,
	}
	for {
		var results []*vfs.Version
		if err := couchdb.GetAllDocs(inst, consts.FilesVersions, &req, &results); err != nil {
			return nil, err
		}
		if len(results) == 0 {
			break
		}
		for _, res := range results {
			if res.DocID == end.ID {
				return versions, nil
			}
			versions = append(versions, res)
		}
		req.StartKeyDocID = results[len(results)-1].DocID
		req.Skip = 1 // Do not fetch again the last file from this page
	}

	return versions, nil
}
