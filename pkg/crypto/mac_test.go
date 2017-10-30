package crypto

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testStrings = []string{"foo", "bar", "baz"}

func TestMACMessageWithName(t *testing.T) {
	value := []byte("myvalue")

	o1 := &MACConfig{
		Key:  []byte("0123456789012345"),
		Name: "message1",
	}
	o2 := &MACConfig{
		Key:  []byte("0123456789012345"),
		Name: "",
	}
	o3 := &MACConfig{
		Key:  []byte("9876543210987654"),
		Name: "message1",
	}

	encoded, err1 := EncodeAuthMessage(o1, value, nil)
	if !assert.NoError(t, err1) {
		return
	}
	v, err2 := DecodeAuthMessage(o1, encoded, nil)
	if !assert.NoError(t, err2) {
		return
	}
	if !assert.EqualValues(t, v, value) {
		t.Fatalf("Expected %v, got %v.", v, value)
	}
	_, err3 := DecodeAuthMessage(o2, encoded, nil)
	if !assert.Error(t, err3) {
		return
	}
	_, err4 := DecodeAuthMessage(o3, encoded, nil)
	if !assert.Error(t, err4) {
		return
	}
	_, err5 := DecodeAuthMessage(o1, encoded, []byte("plop"))
	if !assert.Error(t, err5) {
		return
	}

	encoded2, err6 := EncodeAuthMessage(o1, value, []byte("foo"))
	if !assert.NoError(t, err6) {
		return
	}

	_, err7 := DecodeAuthMessage(o1, encoded2, []byte("foo"))
	if !assert.NoError(t, err7) {
		return
	}
	_, err8 := DecodeAuthMessage(o1, encoded2, []byte("plop"))
	if !assert.Error(t, err8) {
		return
	}
}

func TestMACMessageWithoutName(t *testing.T) {
	value := []byte("myvalue")

	o1 := &MACConfig{
		Key:  []byte("0123456789012345"),
		Name: "",
	}
	o2 := &MACConfig{
		Key:  []byte("0123456789012345"),
		Name: "message1",
	}
	o3 := &MACConfig{
		Key:  []byte("9876543210987654"),
		Name: "",
	}

	encoded, err1 := EncodeAuthMessage(o1, value, nil)
	if !assert.NoError(t, err1) {
		return
	}
	v, err2 := DecodeAuthMessage(o1, encoded, nil)
	if !assert.NoError(t, err2) {
		return
	}
	if !reflect.DeepEqual(v, value) {
		t.Fatalf("Expected %v, got %v.", value, v)
	}
	_, err3 := DecodeAuthMessage(o2, encoded, nil)
	if !assert.Error(t, err3) {
		return
	}
	_, err4 := DecodeAuthMessage(o3, encoded, nil)
	if !assert.Error(t, err4) {
		return
	}
}

func TestMACWrongMessage(t *testing.T) {
	key := []byte("0123456789012345")
	o := &MACConfig{
		Key: key,
	}

	buf1 := new(bytes.Buffer)
	_, err1 := DecodeAuthMessage(o, buf1.Bytes(), nil)
	if !assert.Equal(t, errMACInvalid, err1) {
		return
	}

	buf2 := Base64Encode(GenerateRandomBytes(32))
	_, err2 := DecodeAuthMessage(o, buf2, nil)
	if !assert.Equal(t, errMACInvalid, err2) {
		return
	}

	buf3 := Base64Encode(createMAC(key, []byte("")))
	_, err3 := DecodeAuthMessage(o, buf3, nil)
	if !assert.Equal(t, errMACInvalid, err3) {
		return
	}
}

func TestAuthentication(t *testing.T) {
	hashKey := []byte("secret-key")
	for _, value := range testStrings {
		mac := createMAC(hashKey, []byte(value))
		if !assert.Len(t, mac, macLen) {
			return
		}
		ok1 := verifyMAC(hashKey, []byte(value), mac)
		if !assert.True(t, ok1) {
			return
		}
		ok2 := verifyMAC(hashKey, GenerateRandomBytes(32), mac)
		if !assert.False(t, ok2) {
			return
		}
	}
}
