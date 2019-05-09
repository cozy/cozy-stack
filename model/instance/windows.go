// +build windows

package instance

import "strings"

// DirName returns the name of the subdirectory where instance data are stored.
// On Windows, it's a modified version of the domain name, since some
// characters are forbidden in directory names.
func (i *Instance) DirName() string {
	return strings.Replace(i.Domain, ":", "_", -1)
}
