package revision

import (
	"strconv"
	"strings"
)

// Generation returns the number before the hyphen, called the generation of a
// revision.
func Generation(rev string) int {
	parts := strings.SplitN(rev, "-", 2)
	gen, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return gen
}
