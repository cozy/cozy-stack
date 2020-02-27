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
	section  = []byte("[InternetShortcut]")
	urlField = []byte("URL")
)

// Parse extracts information from a .url file.
func Parse(r io.Reader) (Result, error) {
	var result Result
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return result, err
	}
	lines := bytes.Split(buf, []byte{'\n'})
	firstLine := bytes.TrimSuffix(lines[0], []byte{'\r'})
	if len(lines) < 2 || !bytes.Equal(firstLine, section) {
		return result, ErrInvalidShortcut
	}
	for _, line := range lines[1:] {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		parts := bytes.SplitN(line, []byte{'='}, 2)
		if len(parts) == 2 && bytes.Equal(parts[0], urlField) {
			result.URL = string(parts[1])
		}
	}
	return result, nil
}

// Generate creates the content of a .url file for the given destination URL.
func Generate(url string) []byte {
	u := []byte(url)
	n := len(section) + 2 + len(urlField) + 1 + len(u) + 2
	buf := make([]byte, n)
	i := 0
	copy(buf[i:i+len(section)], section)
	i += len(section)
	buf[i] = '\r'
	i++
	buf[i] = '\n'
	i++
	copy(buf[i:i+len(urlField)], urlField)
	i += len(urlField)
	buf[i] = '='
	i++
	copy(buf[i:i+len(u)], u)
	i += len(u)
	buf[i] = '\r'
	i++
	buf[i] = '\n'
	return buf
}
