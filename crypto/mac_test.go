package crypto

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testStrings = []string{"foo", "bar", "baz"}

func TestMACMessageWithName(t *testing.T) {
	value := "myvalue"

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

	encoded, err1 := EncodeAuthMessage(o1, value)
	if !assert.NoError(t, err1) {
		return
	}
	var dst string
	err2 := DecodeAuthMessage(o1, encoded, &dst)
	if !assert.NoError(t, err2) {
		return
	}
	if !assert.EqualValues(t, dst, value) {
		t.Fatalf("Expected %v, got %v.", value, dst)
	}
	var dst2 string
	err3 := DecodeAuthMessage(o2, encoded, &dst2)
	if !assert.Error(t, err3) {
		return
	}
	err4 := DecodeAuthMessage(o3, encoded, &dst2)
	if !assert.Error(t, err4) {
		return
	}
}

func TestMACMessageWithoutName(t *testing.T) {
	value := "myvalue"

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

	encoded, err1 := EncodeAuthMessage(o1, value)
	if !assert.NoError(t, err1) {
		return
	}
	var dst string
	err2 := DecodeAuthMessage(o1, encoded, &dst)
	if !assert.NoError(t, err2) {
		return
	}
	if !reflect.DeepEqual(dst, value) {
		t.Fatalf("Expected %v, got %v.", value, dst)
	}
	var dst2 string
	err3 := DecodeAuthMessage(o2, encoded, &dst2)
	if !assert.Error(t, err3) {
		return
	}
	err4 := DecodeAuthMessage(o3, encoded, &dst2)
	if !assert.Error(t, err4) {
		return
	}
}

func TestMACWrongMessage(t *testing.T) {
	key := []byte("0123456789012345")
	o := &MACConfig{
		Key: key,
	}

	var dst string

	buf1 := new(bytes.Buffer)
	err1 := DecodeAuthMessage(o, buf1.Bytes(), &dst)
	if !assert.Equal(t, errMACInvalid, err1) {
		return
	}

	buf2 := base64Encode(GenerateRandomBytes(32))
	err2 := DecodeAuthMessage(o, buf2, &dst)
	if !assert.Equal(t, errMACInvalid, err2) {
		return
	}

	buf3 := base64Encode(createMAC(key, []byte("")))
	err3 := DecodeAuthMessage(o, buf3, &dst)
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

func TestEncoding(t *testing.T) {
	for _, value := range testStrings {
		encoded := base64Encode([]byte(value))
		decoded, err := base64Decode(encoded)
		if !assert.NoError(t, err) {
			return
		}
		if !assert.Equal(t, value, string(decoded)) {
			return
		}
	}
}

func TestCustomType(t *testing.T) {
	type MyStruct struct {
		Field1 int64  `json:"field_1"`
		Field2 string `json:"field_2"`
		Field3 []byte `json:"field_3"`
	}

	o := &MACConfig{
		Key: []byte("0123456789012345"),
	}
	src := &MyStruct{42, "bar", []byte("123123123123")}
	encoded, _ := EncodeAuthMessage(o, src)

	dst := &MyStruct{}
	err := DecodeAuthMessage(o, encoded, dst)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, dst.Field1, int64(42))
	assert.Equal(t, dst.Field2, "bar")
	assert.EqualValues(t, dst.Field3, []byte("123123123123"))
}
