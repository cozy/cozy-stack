package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/golang-jwt/jwt/v5"
)

// ErrInstanceAuthenticationFailed is returned when OIDC claims cannot be
// interpreted (missing/malformed sub or user info field). It signals an
// unauthenticated request, not a mismatch.
var ErrInstanceAuthenticationFailed = errors.New("the authentication has failed")

// InstanceMismatchError is returned when an OIDC token does not match the
// target Cozy instance.
type InstanceMismatchError struct {
	ExpectedDomain string
	ActualDomain   string
}

// TranslationKey returns the i18n key for translating this error.
func (e *InstanceMismatchError) TranslationKey() string {
	return "OIDC Domain Mismatch %s %s"
}

// TranslationArgs returns the arguments for the translation.
func (e *InstanceMismatchError) TranslationArgs() []interface{} {
	return []interface{}{e.ExpectedDomain, e.ActualDomain}
}

func (e *InstanceMismatchError) Error() string {
	return fmt.Sprintf("OIDC Domain Mismatch %s %s", e.ExpectedDomain, e.ActualDomain)
}

// CheckClaimsForInstance verifies that the given OIDC claims map to the
// target instance.
func CheckClaimsForInstance(conf *Config, inst *instance.Instance, claims jwt.MapClaims) error {
	if conf.AllowCustomInstance {
		expected := inst.OIDCID
		if conf.Provider == FranceConnectProvider {
			expected = inst.FranceConnectID
		}
		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			inst.Logger().WithNamespace("oidc").
				Errorf("Invalid sub claim: %v != %s", claims["sub"], expected)
			return ErrInstanceAuthenticationFailed
		}
		if sub != expected {
			inst.Logger().WithNamespace("oidc").
				Errorf("Invalid sub: %s != %s", sub, expected)
			return &InstanceMismatchError{ExpectedDomain: inst.Domain, ActualDomain: sub}
		}
		return nil
	}

	domain, err := ExtractDomainFromClaims(conf, claims)
	if err != nil {
		inst.Logger().WithNamespace("oidc").Warnf("Cannot extract domain: %s", err)
		return err
	}
	if domain != inst.Domain {
		inst.Logger().WithNamespace("oidc").
			Errorf("Invalid domains: %s != %s", domain, inst.Domain)
		return &InstanceMismatchError{ExpectedDomain: inst.Domain, ActualDomain: domain}
	}
	return nil
}

// ResolveInstanceFromClaims returns the Cozy instance that the given OIDC
// claims authenticate against.
func ResolveInstanceFromClaims(contextName string, conf *Config, claims jwt.MapClaims) (*instance.Instance, error) {
	if conf.AllowCustomInstance {
		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			logger.WithNamespace("oidc").
				Errorf("Invalid sub claim for context %s: %v", contextName, claims["sub"])
			return nil, ErrInstanceAuthenticationFailed
		}
		return FindInstanceBySub(sub, contextName)
	}

	domain, err := ExtractDomainFromClaims(conf, claims)
	if err != nil {
		logger.WithNamespace("oidc").
			Warnf("Cannot extract domain for context %s: %s", contextName, err)
		return nil, err
	}
	return lifecycle.GetInstance(domain)
}

// ExtractDomainFromClaims reads the configured user info field from the
// claims and applies the prefix/suffix transformation to produce an instance
// domain.
func ExtractDomainFromClaims(conf *Config, claims jwt.MapClaims) (string, error) {
	domain, ok := claims[conf.UserInfoField].(string)
	if !ok {
		return "", ErrInstanceAuthenticationFailed
	}
	return buildInstanceDomain(domain, conf), nil
}

// buildInstanceDomain normalizes the raw user info value into a Cozy
// instance domain.
func buildInstanceDomain(domain string, conf *Config) string {
	domain = strings.ToLower(domain)
	domain = strings.ReplaceAll(domain, "-", "")
	if conf.UserInfoSuffix != "" {
		domain = strings.ReplaceAll(domain, ".", "")
	}
	domain = conf.UserInfoPrefix + domain + conf.UserInfoSuffix
	return domain
}

// FindInstanceBySub returns the instance whose `oidc_id` matches the given
// OIDC subject in the given context, using the `by-oidcid` index.
func FindInstanceBySub(sub, contextName string) (*instance.Instance, error) {
	var instances []*instance.Instance
	req := &couchdb.FindRequest{
		UseIndex: "by-oidcid",
		Selector: mango.And(
			mango.Equal("oidc_id", sub),
			mango.Equal("context", contextName),
		),
		Limit: 1,
	}
	err := couchdb.FindDocs(prefixer.GlobalPrefixer, consts.Instances, req, &instances)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, errors.New("instance not found")
	}
	return instances[0], nil
}
