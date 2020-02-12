// Package shortcut can be used to manipulate files in the .url format (from
// windows). See
// http://www.lyberty.com/encyc/articles/tech/dot_url_format_-_an_unofficial_guide.html
package shortcut

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
)

// ErrInvalidShortcut is the error when a .url file cannot be parsed.
var ErrInvalidShortcut = errors.New("The file is not in the expected format")

// Result is the result of the parsing of a .url file.
type Result struct {
	URL string
}

var (
	section  = []byte("[InternetShortcut]\r\n")
	urlField = []byte("URL")
)

// Parse extracts information from a .url file.
func Parse(r io.Reader) (Result, error) {
	var result Result
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return result, err
	}
	if len(buf) < len(section) || bytes.Compare(buf[:len(section)], section) != 0 {
		return result, ErrInvalidShortcut
	}
	buf = buf[len(section):]
	lines := bytes.Split(buf, []byte("\r"))
	for _, line := range lines {
		line = bytes.TrimPrefix(line, []byte("\n"))
		parts := bytes.SplitN(line, []byte("="), 2)
		if len(parts) == 2 && bytes.Compare(parts[0], urlField) == 0 {
			result.URL = string(parts[1])
		}
	}
	return result, nil
}
