package sessions

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/instance"

	"github.com/stretchr/testify/assert"
)

var delegatedInst *instance.Instance

func TestGoodCheckDelegatedJWT(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzcnV0aSIsIm5hbWUiOiJleHRlcm5hbC5ub3RteWNvenkuY29tIiwiaWF0IjoxNTE2MjM5MDIyLCJleHAiOjE2MDc3MzEyMDAsImVtYWlsIjoic3J1dGlAYWMtcmVubmVzLmZyIiwiY29kZSI6InN0dWRlbnQifQ.mHYke9WhLeggCmv7RdqoAWtQVT45KwT3bz_-fPMcuMc"
	err := CheckDelegatedJWT(delegatedInst, token)
	assert.NoError(t, err)
}

func TestBadExpiredCheckDelegatedJWT(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzcnV0aSIsIm5hbWUiOiJleHRlcm5hbC5ub3RteWNvenkuY29tIiwiaWF0IjoxNTE2MjM5MDIyLCJleHAiOjE1NDg4NDM1NTEsImVtYWlsIjoic3J1dGlAYWMtcmVubmVzLmZyIiwiY29kZSI6InN0dWRlbnQifQ.MqX_DJvLfvMjmZJdgKY006DEjnVGRJRFwmN5Icf6SIk"
	err := CheckDelegatedJWT(delegatedInst, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestBadIssuerCheckDelegatedJWT(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzcnV0aSIsIm5hbWUiOiJleHRlcm5hbC5ub3RteWNvenkubmV0IiwiaWF0IjoxNTE2MjM5MDIyLCJleHAiOjE2MDc3MzEyMDAsImVtYWlsIjoic3J1dGlAYWMtcmVubmVzLmZyIiwiY29kZSI6InN0dWRlbnQifQ.3_bEEEXlSgRDgbAGDnkEu2cDpaF9X8BUf8QmBJH1axI"
	err := CheckDelegatedJWT(delegatedInst, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Issuer")
}
