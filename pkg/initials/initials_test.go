package initials

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetInitials(t *testing.T) {
	assert.Equal(t, "?", getInitials("  "))
	assert.Equal(t, "P", getInitials("Pierre"))
	assert.Equal(t, "FP", getInitials("François Pignon"))
	assert.Equal(t, "П", getInitials("Пьер"))
	assert.Equal(t, "ÉC", getInitials("Éric de la Composition"))
}

func TestGetColor(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	assert.Equal(t, colors[0], getColor(""))

	for i := 0; i < 100; i++ {
		str := randomString()
		color := getColor(str)
		assert.Len(t, color, 7)
		assert.Equal(t, "#", color[0:1])
	}
}

var chars = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZÅÄÖ" +
	"abcdefghijklmnopqrstuvwxyzåäö" +
	"0123456789")

func randomString() string {
	length := 1 + rand.Intn(16)
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}
