//go:generate wget https://raw.githubusercontent.com/sourcegraph/checkup/master/httpchecker.go -O httpchecker.go
//go:generate wget https://raw.githubusercontent.com/sourcegraph/checkup/master/checkup.go -O checkup.go

// package checkup is a subset of the checkup project.
//
// Checkup is a nice project, but it depends on a very large library for AWS
// that we don't use. To avoid the cost of this library, we just use a subset
// of checkup, by just downloading the 2 files that interest us and mocking
// some structs.
//
// See https://github.com/sourcegraph/checkup

package checkup

import "errors"

// S3 is a fake struct to make checkup.go compile
type S3 struct{}

// Store is not implemented
func (s S3) Store(results []Result) error {
	return errors.New("Not implemented")
}

// FS is a fake struct to make checkup.go compile
type FS struct{}

// Store is not implemented
func (fs FS) Store(results []Result) error {
	return errors.New("Not implemented")
}

// TCPChecker is a fake struct to make checkup.go compile
type TCPChecker struct{}

// Check is not implemented
func (c TCPChecker) Check() (Result, error) {
	return Result{}, errors.New("Not implemented")
}

// DNSChecker is a fake struct to make checkup.go compile
type DNSChecker struct{}

// Check is not implemented
func (c DNSChecker) Check() (Result, error) {
	return Result{}, errors.New("Not implemented")
}
