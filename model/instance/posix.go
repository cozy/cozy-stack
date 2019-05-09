// +build !windows

package instance

// DirName returns the name of the subdirectory where instance data are stored.
// On Posix systems, it's the instance domain name.
func (i *Instance) DirName() string {
	return i.Domain
}
