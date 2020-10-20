package move

import (
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/vfs"
)

// Cursor can be used to know which files must be included in a part.
type Cursor struct {
	number int
	index  indexCursor
}

// Number returns the part number associated to this cursor.
func (c Cursor) Number() int {
	return c.number
}

// ParseCursor checks that the given cursor is part of our pre-defined list of
// cursors.
func ParseCursor(exportDoc *ExportDoc, cursorStr string) (Cursor, error) {
	if cursorStr == "" {
		return Cursor{0, indexCursor{}}, nil
	}
	for i, c := range exportDoc.PartsCursors {
		if c == cursorStr {
			parsed, err := parseCursor(c)
			if err != nil {
				return Cursor{}, err
			}
			return Cursor{i + 1, parsed}, nil
		}
	}
	return Cursor{}, ErrExportInvalidCursor
}

// TODO delete code after this comment
type indexCursor struct {
	dirCursor      []int
	fileCursor     int
	fileRangeStart int64
}

type fileRanged struct {
	file       *vfs.TreeFile
	rangeStart int64
	rangeEnd   int64
}

func (c indexCursor) diff(d indexCursor) int {
	l := len(d.dirCursor)
	if len(c.dirCursor) < l {
		l = len(c.dirCursor)
	}
	for i := 0; i < l; i++ {
		if diff := d.dirCursor[i] - c.dirCursor[i]; diff != 0 {
			return diff
		}
	}
	if len(d.dirCursor) > len(c.dirCursor) {
		return 1
	} else if len(d.dirCursor) < len(c.dirCursor) {
		return -1
	}
	return 0
}

func (c indexCursor) equal(d indexCursor) bool {
	l := len(d.dirCursor)
	if l != len(c.dirCursor) {
		return false
	}
	for i := 0; i < l; i++ {
		if d.dirCursor[i] != c.dirCursor[i] {
			return false
		}
	}
	return true
}

func (c indexCursor) next(dirIndex int) (next indexCursor) {
	next.dirCursor = append(c.dirCursor, dirIndex)
	next.fileCursor = 0
	next.fileRangeStart = 0
	return
}

func parseCursor(cursor string) (c indexCursor, err error) {
	if cursor == "" {
		return
	}
	ss := strings.Split(cursor, "/")
	if len(ss) < 2 {
		err = ErrExportInvalidCursor
		return
	}
	if ss[0] != "" {
		err = ErrExportInvalidCursor
		return
	}
	ss = ss[1:]
	c.dirCursor = make([]int, len(ss)-1)
	for i, s := range ss {
		if i == len(ss)-1 {
			rangeSplit := strings.SplitN(s, ":", 2)
			if len(rangeSplit) != 2 {
				err = ErrExportInvalidCursor
				return
			}
			c.fileCursor, err = strconv.Atoi(rangeSplit[0])
			if err != nil {
				return
			}
			c.fileRangeStart, err = strconv.ParseInt(rangeSplit[1], 10, 64)
			if err != nil {
				return
			}
		} else {
			c.dirCursor[i], err = strconv.Atoi(s)
			if err != nil {
				return
			}
		}
	}
	return
}

// splitFilesIndex devides the index into equal size bucket of maximum size
// `bucketSize`. Files can be splitted into multiple parts to accommodate the
// bucket size, using a range. It is used to be able to download the files into
// separate chunks.
//
// The method returns a list of cursor into the index tree for each beginning
// of a new bucket. A cursor has the following format:
//
//    ${dirname}/../${filename}-${byterange-start}
func splitFilesIndex(root *vfs.TreeFile, cursor []string, cursors []string, bucketSize, sizeLeft int64) ([]string, int64) {
	for childIndex, child := range root.FilesChildren {
		size := child.ByteSize
		if size <= sizeLeft {
			sizeLeft -= size
			continue
		}
		size -= sizeLeft
		for size > 0 {
			rangeStart := (child.ByteSize - size)
			cursorStr := strings.Join(append(cursor, strconv.Itoa(childIndex)), "/")
			cursorStr += ":" + strconv.FormatInt(rangeStart, 10)
			cursorStr = "/" + cursorStr
			cursors = append(cursors, cursorStr)
			size -= bucketSize
		}
		sizeLeft = -size
	}
	for dirIndex, dir := range root.DirsChildren {
		cursors, sizeLeft = splitFilesIndex(dir, append(cursor, strconv.Itoa(dirIndex)),
			cursors, bucketSize, sizeLeft)
	}
	return cursors, sizeLeft
}

// listFilesIndex browse the index with the given cursor and returns the
// flatting list of file entering the bucket.
func listFilesIndex(root *vfs.TreeFile, list []fileRanged, currentCursor, cursor indexCursor, bucketSize, sizeLeft int64) ([]fileRanged, int64) {
	if sizeLeft < 0 {
		return list, sizeLeft
	}

	cursorDiff := cursor.diff(currentCursor)
	cursorEqual := cursorDiff == 0 && currentCursor.equal(cursor)

	if cursorDiff >= 0 {
		for childIndex, child := range root.FilesChildren {
			var fileRangeStart, fileRangeEnd int64
			if cursorEqual {
				if childIndex < cursor.fileCursor {
					continue
				} else if childIndex == cursor.fileCursor {
					fileRangeStart = cursor.fileRangeStart
				}
			}
			if sizeLeft <= 0 {
				return list, sizeLeft
			}
			size := child.ByteSize - fileRangeStart
			if sizeLeft-size < 0 {
				fileRangeEnd = fileRangeStart + sizeLeft
			} else {
				fileRangeEnd = child.ByteSize
			}
			list = append(list, fileRanged{child, fileRangeStart, fileRangeEnd})
			sizeLeft -= size
			if sizeLeft < 0 {
				return list, sizeLeft
			}
		}

		// append empty directory so that we explicitly create them in the tarball
		if len(root.DirsChildren) == 0 && len(root.FilesChildren) == 0 {
			list = append(list, fileRanged{root, 0, 0})
		}
	}

	for dirIndex, dir := range root.DirsChildren {
		list, sizeLeft = listFilesIndex(dir, list, currentCursor.next(dirIndex),
			cursor, bucketSize, sizeLeft)
	}

	return list, sizeLeft
}
