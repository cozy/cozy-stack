package crypto

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testStrings = []string{"foo", "bar", "baz"}

func TestMACMessageWithName(t *testing.T) {
	value := []byte("myvalue")

	k1 := []byte("0123456789012345")
	k2 := []byte("0123456789012345")
	k3 := []byte("9876543210987654")
	o1 := MACConfig{Name: "message1"}
	o2 := MACConfig{Name: ""}
	o3 := MACConfig{Name: "message1"}

	encoded, err1 := EncodeAuthMessage(o1, k1, value, nil)
	require.NoError(t, err1)

	v, err2 := DecodeAuthMessage(o1, k1, encoded, nil)
	require.NoError(t, err2)

	if !assert.EqualValues(t, v, value) {
		t.Fatalf("Expected %v, got %v.", v, value)
	}
	_, err3 := DecodeAuthMessage(o2, k2, encoded, nil)
	require.Error(t, err3)

	_, err4 := DecodeAuthMessage(o3, k3, encoded, nil)
	require.Error(t, err4)

	_, err5 := DecodeAuthMessage(o1, k1, encoded, []byte("plop"))
	require.Error(t, err5)

	encoded2, err6 := EncodeAuthMessage(o1, k1, value, []byte("foo"))
	require.NoError(t, err6)

	_, err7 := DecodeAuthMessage(o1, k1, encoded2, []byte("foo"))
	require.NoError(t, err7)

	_, err8 := DecodeAuthMessage(o1, k1, encoded2, []byte("plop"))
	require.Error(t, err8)
}

func TestMACMessageWithoutName(t *testing.T) {
	value := []byte("myvalue")

	k1 := []byte("0123456789012345")
	k2 := []byte("0123456789012345")
	k3 := []byte("9876543210987654")
	o1 := MACConfig{Name: ""}
	o2 := MACConfig{Name: "message1"}
	o3 := MACConfig{Name: ""}

	encoded, err1 := EncodeAuthMessage(o1, k1, value, nil)
	require.NoError(t, err1)

	v, err2 := DecodeAuthMessage(o1, k1, encoded, nil)
	require.NoError(t, err2)

	if !reflect.DeepEqual(v, value) {
		t.Fatalf("Expected %v, got %v.", value, v)
	}
	_, err3 := DecodeAuthMessage(o2, k2, encoded, nil)
	require.Error(t, err3)

	_, err4 := DecodeAuthMessage(o3, k3, encoded, nil)
	require.Error(t, err4)
}

func TestMACWrongMessage(t *testing.T) {
	k := []byte("0123456789012345")
	o := MACConfig{
		Name:   "name",
		MaxLen: 256,
	}

	{
		_, err := DecodeAuthMessage(o, k, []byte(""), nil)
		require.Equal(t, errMACInvalid, err)
	}

	{
		_, err := DecodeAuthMessage(o, k, []byte("ccc"), nil)
		require.Equal(t, errMACInvalid, err)
	}

	{
		buf := Base64Encode(GenerateRandomBytes(32))
		_, err := DecodeAuthMessage(o, k, buf, nil)
		require.Equal(t, errMACInvalid, err)
	}

	{
		buf := Base64Encode(createMAC(k, []byte("")))
		_, err := DecodeAuthMessage(o, k, buf, nil)
		require.Equal(t, errMACInvalid, err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10000; i++ {
		buf := Base64Encode(GenerateRandomBytes(rng.Intn(1000)))
		_, err := DecodeAuthMessage(o, k, buf, nil)
		require.Equal(t, errMACInvalid, err)
	}
}

func TestMACMaxAge(t *testing.T) {
	key := GenerateRandomBytes(16)
	val := []byte("coucou")
	add := []byte("additional")
	c1 := MACConfig{
		Name:   "max",
		MaxAge: 1 * time.Second,
	}
	c2 := MACConfig{
		Name:   "max",
		MaxAge: 10 * time.Second,
	}

	var msg1, msg2 []byte

	{
		var err error
		msg1, err = EncodeAuthMessage(c1, key, val, add)
		require.NoError(t, err)

		msg2, err = EncodeAuthMessage(c2, key, val, add)
		require.NoError(t, err)
	}

	{
		ret1, err := DecodeAuthMessage(c1, key, msg1, add)
		require.NoError(t, err)

		assert.Equal(t, val, ret1)

		ret2, err := DecodeAuthMessage(c2, key, msg2, add)
		require.NoError(t, err)

		assert.Equal(t, val, ret2)
	}

	time.Sleep(1500 * time.Millisecond)

	{
		ret1, err := DecodeAuthMessage(c1, key, msg1, add)
		assert.Error(t, err)
		assert.Nil(t, ret1)
		assert.Equal(t, errMACExpired, err)

		ret2, err := DecodeAuthMessage(c2, key, msg2, add)
		assert.NoError(t, err)
		assert.Equal(t, val, ret2)
	}
}

func TestAuthentication(t *testing.T) {
	hashKey := []byte("secret-key")
	for _, value := range testStrings {
		mac := createMAC(hashKey, []byte(value))
		require.Len(t, mac, macLen)

		ok1 := verifyMAC(hashKey, []byte(value), mac)
		require.True(t, ok1)

		ok2 := verifyMAC(hashKey, GenerateRandomBytes(32), mac)
		require.False(t, ok2)
	}
}
