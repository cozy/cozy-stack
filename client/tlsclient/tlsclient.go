package tlsclient

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/pkg/utils"
)

type HTTPEndpoint struct {
	Host      string
	Port      int
	Timeout   time.Duration
	EnvPrefix string

	RootCAFile             string
	ClientCertificateFiles ClientCertificateFilePair
	PinnedKey              string
	InsecureSkipValidation bool
}

type ClientCertificateFilePair struct {
	KeyFile         string
	CertificateFile string
}

type tlsConfig struct {
	clientCertificates []tls.Certificate
	rootCAs            []*x509.Certificate
	pinnedKeys         [][]byte
	skipVerification   bool
}

func generateURL(host string, port int) (*url.URL, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u = &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		}
	}
	return u, nil
}

func NewHTTPClient(opt HTTPEndpoint) (client *http.Client, u *url.URL, err error) {
	if opt.Host != "" || opt.Port > 0 {
		u, err = generateURL(opt.Host, opt.Port)
		if err != nil {
			return
		}
	}
	c := &tlsConfig{}
	if u != nil {
		c, u, err = fromURL(c, u)
		if err != nil {
			return
		}
	}
	if opt.EnvPrefix != "" {
		c, err = fromEnv(c, opt.EnvPrefix)
		if err != nil {
			return
		}
	}
	if opt.RootCAFile != "" {
		if err = c.LoadRootCAFile(opt.RootCAFile); err != nil {
			return
		}
	}
	if opt.ClientCertificateFiles.CertificateFile != "" {
		if err = c.LoadClientCertificateFile(
			opt.ClientCertificateFiles.CertificateFile,
			opt.ClientCertificateFiles.KeyFile,
		); err != nil {
			return
		}
	}
	if opt.PinnedKey != "" {
		if err = c.AddHexPinnedKey(opt.PinnedKey); err != nil {
			return
		}
	}
	if opt.InsecureSkipValidation {
		c.SetInsecureSkipValidation()
	}
	client = &http.Client{
		Timeout:   opt.Timeout,
		Transport: &http.Transport{TLSClientConfig: c.Config()},
	}
	return
}

func fromURL(c *tlsConfig, u *url.URL) (conf *tlsConfig, uCopy *url.URL, err error) {
	uCopy = utils.CloneURL(u)
	q := uCopy.Query()
	if u.Scheme == "https" {
		if rootCAFile := q.Get("ca"); rootCAFile != "" {
			if err = c.LoadRootCAFile(rootCAFile); err != nil {
				return
			}
		}
		if certFile := q.Get("cert"); certFile != "" {
			if keyFile := q.Get("key"); keyFile != "" {
				if err = c.LoadClientCertificateFile(certFile, keyFile); err != nil {
					return
				}
			}
		}
		if hexPinnedKey := q.Get("fp"); hexPinnedKey != "" {
			if err = c.AddHexPinnedKey(hexPinnedKey); err != nil {
				return
			}
		}
		if t := q.Get("validate"); t == "0" || t == "false" || t == "FALSE" {
			c.SetInsecureSkipValidation()
		}
	}
	q.Del("ca")
	q.Del("cert")
	q.Del("key")
	q.Del("fp")
	q.Del("validate")
	uCopy.RawQuery = q.Encode()
	return c, uCopy, nil
}

func fromEnv(c *tlsConfig, envPrefix string) (conf *tlsConfig, err error) {
	if rootCAFile := os.Getenv(envPrefix + "_CA"); rootCAFile != "" {
		if err = c.LoadRootCAFile(rootCAFile); err != nil {
			return
		}
	}
	if certFile := os.Getenv(envPrefix + "_CERT"); certFile != "" {
		if keyFile := os.Getenv(envPrefix + "_KEY"); keyFile != "" {
			if err = c.LoadClientCertificateFile(certFile, keyFile); err != nil {
				return
			}
		}
	}
	if hexPinnedKey := os.Getenv(envPrefix + "_FINGERPRINT"); hexPinnedKey != "" {
		if err = c.AddHexPinnedKey(hexPinnedKey); err != nil {
			return
		}
	}
	if t := os.Getenv(envPrefix + "_VALIDATE"); t == "0" || t == "false" || t == "FALSE" {
		c.SetInsecureSkipValidation()
	}
	return c, nil
}

func (s *tlsConfig) LoadClientCertificateFile(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("tlsclient: could not load client certificate file: %s", err)
	}
	s.clientCertificates = append(s.clientCertificates, cert)
	return nil
}

func (s *tlsConfig) LoadClientCertificate(certPEMBlock, keyPEMBlock []byte) error {
	cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return fmt.Errorf("tlsclient: could not load client certificate file: %s", err)
	}
	s.clientCertificates = append(s.clientCertificates, cert)
	return nil
}

func (s *tlsConfig) LoadRootCA(rootCA []byte) error {
	cert, err := x509.ParseCertificate(rootCA)
	if err != nil {
		return err
	}
	s.rootCAs = append(s.rootCAs, cert)
	return nil
}

func (s *tlsConfig) LoadRootCAFile(rootCAFile string) error {
	pemCerts, err := ioutil.ReadFile(rootCAFile)
	if err != nil {
		return fmt.Errorf("tlsclient: could not load root CA file %q: %s", rootCAFile, err)
	}
	ok := false
	for len(pemCerts) > 0 {
		var block *pem.Block
		block, pemCerts = pem.Decode(pemCerts)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		if err = s.LoadRootCA(block.Bytes); err != nil {
			continue
		}
		ok = true
	}
	if !ok {
		return fmt.Errorf("tlsclient: could not load any certificate from the given ROOTCA file: %q", rootCAFile)
	}
	return nil
}

func (s *tlsConfig) SetInsecureSkipValidation() {
	s.skipVerification = true
}

func (s *tlsConfig) AddHexPinnedKey(hexPinnedKey string) error {
	pinnedKey, err := hex.DecodeString(hexPinnedKey)
	if err != nil {
		return fmt.Errorf("tlsclient: invalid hexadecimal fingerprint: %s", err)
	}
	expected := sha256.Size
	given := len(pinnedKey)
	if given != expected {
		return fmt.Errorf("tlsclient: invalid fingerprint size for %s, expected %d got %d", hexPinnedKey,
			expected, given)
	}
	s.pinnedKeys = append(s.pinnedKeys, pinnedKey)
	return nil
}

func (s *tlsConfig) Config() *tls.Config {
	conf := &tls.Config{}
	conf.InsecureSkipVerify = s.skipVerification

	if len(s.rootCAs) > 0 {
		rootCAs := x509.NewCertPool()
		for _, cert := range s.rootCAs {
			rootCAs.AddCert(cert)
		}
		conf.RootCAs = rootCAs
	}

	if len(s.clientCertificates) > 0 {
		conf.Certificates = make([]tls.Certificate, len(s.clientCertificates))
		copy(conf.Certificates, s.clientCertificates)
	}

	if len(s.pinnedKeys) > 0 {
		conf.VerifyPeerCertificate = verifyCertificatePinnedKey(s.pinnedKeys)
	}
	return conf
}

func verifyCertificatePinnedKey(pinnedKeys [][]byte) func(certs [][]byte, verifiedChains [][]*x509.Certificate) error {
	return func(certs [][]byte, verifiedChains [][]*x509.Certificate) error {
		// Check for leaf pinning first
		for _, asn1 := range certs {
			cert, err := x509.ParseCertificate(asn1)
			if err != nil {
				return err
			}
			fingerPrint := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
			for _, pinnedKey := range pinnedKeys {
				if bytes.Equal(pinnedKey, fingerPrint[:]) {
					return nil
				}
			}
		}
		// Then check for intermediate pinning
		for _, verifiedChain := range verifiedChains {
			if len(verifiedChain) > 0 {
				verifiedCert := verifiedChain[0]
				fingerPrint := sha256.Sum256(verifiedCert.RawSubjectPublicKeyInfo)
				for _, pinnedKey := range pinnedKeys {
					if bytes.Equal(pinnedKey, fingerPrint[:]) {
						return nil
					}
				}
			}
		}
		return fmt.Errorf("tlsclient: could not find the valid pinned key from proposed ones")
	}
}
