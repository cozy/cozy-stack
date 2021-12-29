package oauth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/cozy/cozy-stack/model/instance"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/ugorji/go/codec"
)

// APPLE_APP_ATTEST_ROOT_CERT is the certificate coming from
// https://www.apple.com/certificateauthority/Apple_App_Attestation_Root_CA.pem
const APPLE_APP_ATTEST_ROOT_CERT = `-----BEGIN CERTIFICATE-----
MIICITCCAaegAwIBAgIQC/O+DvHN0uD7jG5yH2IXmDAKBggqhkjOPQQDAzBSMSYw
JAYDVQQDDB1BcHBsZSBBcHAgQXR0ZXN0YXRpb24gUm9vdCBDQTETMBEGA1UECgwK
QXBwbGUgSW5jLjETMBEGA1UECAwKQ2FsaWZvcm5pYTAeFw0yMDAzMTgxODMyNTNa
Fw00NTAzMTUwMDAwMDBaMFIxJjAkBgNVBAMMHUFwcGxlIEFwcCBBdHRlc3RhdGlv
biBSb290IENBMRMwEQYDVQQKDApBcHBsZSBJbmMuMRMwEQYDVQQIDApDYWxpZm9y
bmlhMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAERTHhmLW07ATaFQIEVwTtT4dyctdh
NbJhFs/Ii2FdCgAHGbpphY3+d8qjuDngIN3WVhQUBHAoMeQ/cLiP1sOUtgjqK9au
Yen1mMEvRq9Sk3Jm5X8U62H+xTD3FE9TgS41o0IwQDAPBgNVHRMBAf8EBTADAQH/
MB0GA1UdDgQWBBSskRBTM72+aEH/pwyp5frq5eWKoTAOBgNVHQ8BAf8EBAMCAQYw
CgYIKoZIzj0EAwMDaAAwZQIwQgFGnByvsiVbpTKwSga0kP0e8EeDS4+sQmTvb7vn
53O5+FRXgeLhpJ06ysC5PrOyAjEAp5U4xDgEgllF7En3VcE3iexZZtKeYnpqtijV
oyFraWVIyd/dganmrduC1bmTBGwD
-----END CERTIFICATE-----`

type appleAttestationObject struct {
	Format       string                 `codec:"fmt"`
	AttStatement map[string]interface{} `codec:"attStmt,omitempty"`
	RawAuthData  []byte                 `codec:"authData"`
	AuthData     authenticatorData
}

// authenticatorData is described by
// https://www.w3.org/TR/webauthn/#sctn-authenticator-data
type authenticatorData struct {
	RPIDHash       []byte
	Flags          authenticatorFlags
	Counter        uint32
	AttestedData   attestedCredentialData
	ExtensionsData []byte
}

type attestedCredentialData struct {
	AAGUID       []byte
	CredentialID []byte
}

type authenticatorFlags byte

func (f authenticatorFlags) Has(flag authenticatorFlags) bool {
	return (f & flag) == flag
}

const (
	flagAttestedCredentialData authenticatorFlags = 64 // Bit 6: Attested credential data included (AT)
)

// checkAppleAttestation will check an attestation made by the DeviceCheck API.
// Cf https://developer.apple.com/documentation/devicecheck/validating_apps_that_connect_to_your_server#3576643
func (c *Client) checkAppleAttestation(inst *instance.Instance, req AttestationRequest) error {
	store := GetStore()
	if ok := store.CheckAndClearChallenge(inst, c.ID(), req.Challenge); !ok {
		return errors.New("invalid challenge")
	}

	obj, err := parseAppleAttestation(req.Attestation)
	if err != nil {
		return fmt.Errorf("cannot parse attestation: %s", err)
	}
	inst.Logger().Debugf("checkAppleAttestation claims = %#v", obj)

	if err := obj.checkCertificate(req.Challenge, req.KeyID); err != nil {
		return err
	}
	if err := obj.checkAttestationData(req.KeyID); err != nil {
		return err
	}
	return nil
}

func parseAppleAttestation(attestation string) (*appleAttestationObject, error) {
	raw, err := base64.StdEncoding.DecodeString(attestation)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64: %s", err)
	}
	obj := appleAttestationObject{}
	cborHandler := codec.CborHandle{}
	err = codec.NewDecoderBytes(raw, &cborHandler).Decode(&obj)
	if err != nil {
		return nil, fmt.Errorf("error decoding cbor: %s", err)
	}
	if obj.Format != "apple-appattest" {
		return nil, errors.New("invalid webauthn format")
	}

	obj.AuthData, err = parseAuthData(obj.RawAuthData)
	if err != nil {
		return nil, fmt.Errorf("error decoding auth data: %v", err)
	}
	if !obj.AuthData.Flags.Has(flagAttestedCredentialData) {
		return nil, errors.New("missing attested credential data flag")
	}
	return &obj, nil
}

// parseAuthData parse webauthn Attestation object.
// Cf https://www.w3.org/TR/webauthn/#sctn-attestation
func parseAuthData(raw []byte) (authenticatorData, error) {
	var data authenticatorData
	if len(raw) < 37 {
		return data, errors.New("raw AuthData is too short")
	}
	data.RPIDHash = raw[:32]
	data.Flags = authenticatorFlags(raw[32])
	data.Counter = binary.BigEndian.Uint32(raw[33:37])
	if len(raw) == 37 {
		return data, nil
	}

	if len(raw) < 55 {
		return data, errors.New("raw AuthData is too short")
	}
	data.AttestedData.AAGUID = raw[37:53]
	idLength := binary.BigEndian.Uint16(raw[53:55])
	if len(raw) < int(55+idLength) {
		return data, errors.New("raw AuthData is too short")
	}
	data.AttestedData.CredentialID = raw[55 : 55+idLength]
	return data, nil
}

func (obj *appleAttestationObject) checkCertificate(challenge string, keyID []byte) error {
	// 1. Verify that the x5c array contains the intermediate and leaf
	// certificates for App Attest, starting from the credential certificate
	// stored in the first data buffer in the array (credcert). Verify the
	// validity of the certificates using Apple’s root certificate.
	credCert, opts, err := obj.setupAppleCertificates()
	if err != nil {
		return err
	}
	if _, err := credCert.Verify(*opts); err != nil {
		return err
	}

	// 2. Create clientDataHash as the SHA256 hash of the one-time challenge
	// sent to your app before performing the attestation, and append that hash
	// to the end of the authenticator data (authData from the decoded object).
	clientDataHash := sha256.Sum256([]byte(challenge))
	composite := append(obj.RawAuthData, clientDataHash[:]...)

	// 3. Generate a new SHA256 hash of the composite item to create nonce.
	nonce := sha256.Sum256(composite)

	// 4. Obtain the value of the credCert extension with OID
	// 1.2.840.113635.100.8.2, which is a DER-encoded ASN.1 sequence. Decode
	// the sequence and extract the single octet string that it contains.
	// Verify that the string equals nonce.
	extracted, err := extractNonceFromCertificate(credCert)
	if err != nil {
		return err
	}
	if !bytes.Equal(nonce[:], extracted) {
		return errors.New("invalid nonce")
	}

	// 5. Create the SHA256 hash of the public key in credCert, and verify that
	// it matches the key identifier from your app.
	pub, ok := credCert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("invalid algorithm for credCert")
	}
	pubKey := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	pubKeyHash := sha256.Sum256(pubKey)
	if !bytes.Equal(pubKeyHash[:], keyID) {
		return errors.New("invalid keyId")
	}
	return nil
}

func (obj *appleAttestationObject) setupAppleCertificates() (*x509.Certificate, *x509.VerifyOptions, error) {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(APPLE_APP_ATTEST_ROOT_CERT))
	if !ok {
		return nil, nil, errors.New("error adding root certificate to pool")
	}

	x5c, ok := obj.AttStatement["x5c"].([]interface{})
	if !ok || len(x5c) == 0 {
		return nil, nil, errors.New("missing certification")
	}

	certs := make([]*x509.Certificate, 0, len(x5c))
	for _, raw := range x5c {
		rawBytes, ok := raw.([]byte)
		if !ok {
			return nil, nil, errors.New("missing certification")
		}
		cert, err := x509.ParseCertificate(rawBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing cert: %s", err)
		}
		certs = append(certs, cert)
	}
	intermediates := x509.NewCertPool()
	for _, cert := range certs {
		intermediates.AddCert(cert)
	}

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	}
	credCert := certs[0]
	return credCert, &opts, nil
}

func extractNonceFromCertificate(credCert *x509.Certificate) ([]byte, error) {
	credCertOID := asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 2}
	var credCertId []byte
	for _, extension := range credCert.Extensions {
		if extension.Id.Equal(credCertOID) {
			credCertId = extension.Value
		}
	}
	if len(credCertId) == 0 {
		return nil, errors.New("missing credCert extension")
	}
	var values []asn1.RawValue
	_, err := asn1.Unmarshal(credCertId, &values)
	if err != nil || len(values) == 0 {
		return nil, errors.New("missing credCert value")
	}
	var value asn1.RawValue
	if _, err = asn1.Unmarshal(values[0].Bytes, &value); err != nil {
		return nil, errors.New("missing credCert value")
	}
	return value.Bytes, nil
}

func (obj *appleAttestationObject) checkAttestationData(keyID []byte) error {
	// 6. Compute the SHA256 hash of your app’s App ID, and verify that this is
	// the same as the authenticator data’s RP ID hash.
	if err := checkAppID(obj.AuthData.RPIDHash); err != nil {
		return err
	}

	// 7. Verify that the authenticator data’s counter field equals 0.
	if obj.AuthData.Counter != 0 {
		return errors.New("invalid counter")
	}

	// 8. Verify that the authenticator data’s aaguid field is either
	// appattestdevelop if operating in the development environment, or
	// appattest followed by seven 0x00 bytes if operating in the production
	// environment.
	aaguid := [16]byte{'a', 'p', 'p', 'a', 't', 't', 'e', 's', 't', 0, 0, 0, 0, 0, 0, 0}
	if build.IsDevRelease() {
		copy(aaguid[:], "appattestdevelop")
	}
	if !bytes.Equal(obj.AuthData.AttestedData.AAGUID, aaguid[:]) {
		return errors.New("invalid aaguid")
	}

	// 9. Verify that the authenticator data’s credentialId field is the same
	// as the key identifier.
	if !bytes.Equal(obj.AuthData.AttestedData.CredentialID, keyID) {
		return errors.New("invalid credentialId")
	}
	return nil
}

func checkAppID(hash []byte) error {
	appIDs := config.GetConfig().Flagship.AppleAppIDs
	for _, appID := range appIDs {
		appIDHash := sha256.Sum256([]byte(appID))
		if bytes.Equal(hash, appIDHash[:]) {
			return nil
		}
	}
	return errors.New("invalid RP ID hash")
}
