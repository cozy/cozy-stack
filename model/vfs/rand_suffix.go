package vfs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Code taken from io/ioutil go package

// Random number state.
// We generate random temporary file names so that there's a good
// chance the file doesn't exist yet - keeps the number of tries in
// TempFile to a minimum.
var rand uint32
var randmu sync.Mutex

func nextSuffix() string {
	randmu.Lock()
	r := rand
	if r == 0 {
		r = reseed()
	}
	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	rand = r
	randmu.Unlock()
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}

func reseed() uint32 {
	return uint32(time.Now().UnixNano() + int64(os.Getpid()))
}

// tryOrUseSuffix will try the given function until it succeed without
// an os.ErrExist error. It is used for renaming safely a file without
// collision.
func tryOrUseSuffix(name, format string, do func(suffix string) error) error {
	var err error
	nconflict := 0
	for i := 0; i < 1000; i++ {
		var newname string
		if i == 0 {
			newname = name
		} else {
			newname = fmt.Sprintf(format, name, nextSuffix())
		}
		err = do(newname)
		if !os.IsExist(err) {
			break
		}
		if nconflict++; nconflict > 10 {
			randmu.Lock()
			rand = reseed()
			randmu.Unlock()
		}
	}
	return err
}

func stripSuffix(name, suffix string) string {
	loc := strings.LastIndex(name, suffix)
	if loc == -1 {
		return name
	}
	return name[:loc]
}
