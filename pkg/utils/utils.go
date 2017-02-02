package utils

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func init() {
	// So that we do not generate the same IDs upon restart
	rand.Seed(time.Now().UTC().UnixNano())
}

// RandomString returns a string of random alpha characters of the specified
// length.
//
// TODO(optim): check the usage of the global locked rng does not become a
// bottleneck.
func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	lenLetters := len(letters)
	for i := 0; i < n; i++ {
		b[i] = letters[rand.Intn(lenLetters)]
	}
	return string(b)
}

// FileExists returns whether or not the file exists on the current file
// system.
func FileExists(name string) (bool, error) {
	infos, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if infos.IsDir() {
		return false, fmt.Errorf("Path %s is a directory", name)
	}
	return true, nil
}

// DirExists returns whether or not the directory exists on the current file
// system.
func DirExists(name string) (bool, error) {
	infos, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !infos.IsDir() {
		return false, fmt.Errorf("Path %s is not a directory", name)
	}
	return true, nil
}

// UserHomeDir returns the user's home directory
func UserHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return os.Getenv("HOME")
}

// AbsPath returns an absolute path relative.
func AbsPath(inPath string) string {
	if strings.HasPrefix(inPath, "~") {
		inPath = UserHomeDir() + inPath[len("~"):]
	} else if strings.HasPrefix(inPath, "$HOME") {
		inPath = UserHomeDir() + inPath[len("$HOME"):]
	}

	if strings.HasPrefix(inPath, "$") {
		end := strings.Index(inPath, string(os.PathSeparator))
		inPath = os.Getenv(inPath[1:end]) + inPath[end:]
	}

	p, err := filepath.Abs(inPath)
	if err == nil {
		return filepath.Clean(p)
	}

	return ""
}
