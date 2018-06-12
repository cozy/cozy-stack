package utils

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

func init() {
	// So that we do not generate the same IDs upon restart
	rand.Seed(time.Now().UTC().UnixNano())
}

// RandomString returns a string of random alpha characters of the specified
// length.
func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	lenLetters := len(letters)
	for i := 0; i < n; i++ {
		b[i] = letters[rand.Intn(lenLetters)]
	}
	return string(b)
}

// RandomStringFast returns a random string containing printable ascii
// characters: [0-9a-zA-Z_-]{n}. Each character encodes 6bits of entropy. To
// avoid wasting entropy, it is better to create a string whose length is a
// multiple of 10. For instance a 20 bytes string will encode 120 bits of
// entropy.
func RandomStringFast(rng *rand.Rand, n int) string {
	// extract 10 letters — 60 bits of entropy — for each pseudo-random uint64
	const K = 10
	const L = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"
	b := make([]byte, ((n+K-1)/K)*K)
	for i := 0; i < n; i += K {
		rn := rng.Uint64()
		for j := 0; j < K; j++ {
			b[i+j] = L[rn&0x3F]
			rn >>= 6
		}
	}
	return string(b[:n])
}

// IsInArray returns whether or not a string is in the given array of strings.
func IsInArray(s string, a []string) bool {
	for _, ss := range a {
		if s == ss {
			return true
		}
	}
	return false
}

// StripPort extract the domain name from a domain:port string.
func StripPort(domain string) string {
	if strings.Contains(domain, ":") {
		cleaned, _, err := net.SplitHostPort(domain)
		if err != nil {
			return domain
		}
		return cleaned
	}
	return domain
}

// SplitTrimString slices s into all substrings a s separated by sep, like
// strings.Split. In addition it will trim all those substrings and filter out
// the empty ones.
func SplitTrimString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	return TrimStrings(strings.Split(s, sep))
}

// TrimStrings trim all strings and filter out the empty ones.
func TrimStrings(strs []string) []string {
	filteredStrs := strs[:0]
	for _, part := range strs {
		part = strings.TrimSpace(part)
		if part != "" {
			filteredStrs = append(filteredStrs, part)
		}
	}
	return filteredStrs
}

// UniqueStrings returns a filtered slice without string duplicates.
func UniqueStrings(strs []string) []string {
	filteredStrs := strs[:0]
	for _, str1 := range strs {
		found := false
		for _, str2 := range filteredStrs {
			if str1 == str2 {
				found = true
				break
			}
		}
		if !found {
			filteredStrs = append(filteredStrs, str1)
		}
	}
	return filteredStrs
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

// CleanUTF8 returns a string with only valid UTF-8 runes
func CleanUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	v := make([]rune, 0, len(s))
	for _, r := range s {
		if r != utf8.RuneError {
			v = append(v, r)
		}
	}
	return string(v)
}

// CloneURL clones the given url
func CloneURL(u *url.URL) *url.URL {
	clone := *u
	if clone.User != nil {
		tmp := *clone.User
		clone.User = &tmp
	}
	return &clone
}
