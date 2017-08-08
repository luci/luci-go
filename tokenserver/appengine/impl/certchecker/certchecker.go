// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package certchecker contains implementation of CertChecker.
//
// CertChecker knows how to check certificate signatures and revocation status.
//
// Uses datastore entities managed by 'certconfig' package.
package certchecker

import (
	"crypto/x509"
	"fmt"
	"time"

	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/data/caching/proccache"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
)

const (
	// RefetchCAPeriod is how often to check CA entity in the datastore.
	//
	// A big value here is acceptable, since CA is changing only when service
	// config is changing (which happens infrequently).
	RefetchCAPeriod = 5 * time.Minute

	// RefetchCRLPeriod is how often to check CRL entities in the datastore.
	//
	// CRL changes pretty frequently, caching it for too long is harmful
	// (increases delay between a cert is revoked by CA and no longer accepted by
	// the token server).
	RefetchCRLPeriod = 15 * time.Second
)

// ErrorReason is part of Error struct.
type ErrorReason int

const (
	// NoSuchCA is returned by GetCertChecker or GetCA if requested CA is not
	// defined in the config.
	NoSuchCA ErrorReason = iota

	// UnknownCA is returned by CheckCertificate if the cert was signed by an
	// unexpected CA (i.e. a CA CertChecker is not configured with).
	UnknownCA

	// NotReadyCA is returned by CheckCertificate if the CA's CRL hasn't been
	// fetched yet (and thus CheckCertificate can't verify certificate's
	// revocation status).
	NotReadyCA

	// CertificateExpired is returned by CheckCertificate if the cert has
	// expired already or not yet active.
	CertificateExpired

	// SignatureCheckError is returned by CheckCertificate if the certificate
	// signature is not valid.
	SignatureCheckError

	// CertificateRevoked is returned by CheckCertificate if the certificate is
	// in the CA's Certificate Revocation List.
	CertificateRevoked
)

// Error is returned by CertChecker methods in case the certificate is invalid.
//
// Datastore errors and not wrapped in Error, but returned as is. You may use
// type cast to Error to distinguish certificate related errors from other kinds
// of errors.
type Error struct {
	error              // inner error with text description
	Reason ErrorReason // enumeration that can be used in switches
}

// NewError instantiates Error.
//
// It is needed because initializing 'error' field on Error is not allowed
// outside of this package (it is lowercase - "unexported").
func NewError(e error, reason ErrorReason) error {
	return Error{e, reason}
}

// IsCertInvalidError returns true for errors from CheckCertificate that
// indicate revoked or expired or otherwise invalid certificates.
//
// Such errors can be safely cast to Error.
func IsCertInvalidError(err error) bool {
	_, ok := err.(Error)
	return ok
}

// CertChecker knows how to check certificate signatures and revocation status.
//
// It is associated with single CA and assumes all certs needing a check are
// signed by that CA directly (i.e. there is no intermediate CAs).
//
// It caches CRL lists internally and must be treated as a heavy global object.
// Use GetCertChecker to grab a global instance of CertChecker for some CA.
//
// CertChecker is safe for concurrent use.
type CertChecker struct {
	CN  string                 // Common Name of the CA
	CRL *certconfig.CRLChecker // knows how to query certificate revocation list

	ca lazyslot.Slot // knows how to load CA cert and config
}

type proccacheKey string

// CheckCertificate checks validity of a given certificate.
//
// It looks at the cert issuer, loads corresponding CertChecker and calls its
// CheckCertificate method. See CertChecker.CheckCertificate documentation for
// explanation of return values.
func CheckCertificate(c context.Context, cert *x509.Certificate) (*certconfig.CA, error) {
	checker, err := GetCertChecker(c, cert.Issuer.CommonName)
	if err != nil {
		return nil, err
	}
	return checker.CheckCertificate(c, cert)
}

// GetCertChecker returns an instance of CertChecker for given CA.
//
// It caches CertChecker objects in local memory and reuses them between
// requests.
func GetCertChecker(c context.Context, cn string) (*CertChecker, error) {
	checker, err := proccache.GetOrMake(c, proccacheKey(cn), func() (interface{}, time.Duration, error) {
		// To avoid storing CertChecker for non-existent CAs in local memory forever,
		// we do a datastore check when creating the checker. It happens once during
		// the process lifetime.
		switch exists, err := ds.Exists(c, ds.NewKey(c, "CA", cn, 0, nil)); {
		case err != nil:
			return nil, 0, transient.Tag.Apply(err)
		case !exists.All():
			return nil, 0, Error{
				error:  fmt.Errorf("no such CA %q", cn),
				Reason: NoSuchCA,
			}
		}
		checker := &CertChecker{
			CN:  cn,
			CRL: certconfig.NewCRLChecker(cn, certconfig.CRLShardCount, refetchCRLPeriod(c)),
		}
		checker.ca.Fetcher = func(c context.Context, _ lazyslot.Value) (lazyslot.Value, error) {
			ca, err := checker.refetchCA(c)
			if err != nil {
				return lazyslot.Value{}, err
			}
			return lazyslot.Value{
				Value:      ca,
				Expiration: clock.Now(c).Add(refetchCAPeriod(c)),
			}, nil
		}
		return checker, 0, nil
	})
	if err != nil {
		return nil, err
	}
	return checker.(*CertChecker), nil
}

// GetCA returns CA entity with ParsedConfig and ParsedCert fields set.
func (ch *CertChecker) GetCA(c context.Context) (*certconfig.CA, error) {
	value, err := ch.ca.Get(c)
	if err != nil {
		return nil, err
	}
	// See Fetcher in GetCertChecker, it puts *certconfig.CA in the ch.ca slot.
	ca := value.Value.(*certconfig.CA)
	// Empty 'ca' means 'refetchCA' could not find it in the datastore. May happen
	// if CA entity was deleted after GetCertChecker call. It could have been also
	// "soft-deleted" by setting Removed == true.
	if ca.CN == "" || ca.Removed {
		return nil, Error{
			error:  fmt.Errorf("no such CA %q", ch.CN),
			Reason: NoSuchCA,
		}
	}
	return ca, nil
}

// CheckCertificate checks certificate's signature, validity period and
// revocation status.
//
// It returns nil error iff cert was directly signed by the CA, not expired yet,
// and its serial number is not in the CA's CRL.
//
// On success also returns *certconfig.CA instance used to check the
// certificate, since 'GetCA' may return another instance (in case certconfig.CA
// cache happened to expire between the calls).
func (ch *CertChecker) CheckCertificate(c context.Context, cert *x509.Certificate) (*certconfig.CA, error) {
	// Has the cert expired already?
	now := clock.Now(c)
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, Error{
			error:  fmt.Errorf("certificate has expired"),
			Reason: CertificateExpired,
		}
	}

	// Grab CA cert from the datastore.
	ca, err := ch.GetCA(c)
	if err != nil {
		return nil, err
	}

	// Did we fetch its CRL at least once?
	if !ca.Ready {
		return nil, Error{
			error:  fmt.Errorf("CRL of CA %q is not ready yet", ch.CN),
			Reason: NotReadyCA,
		}
	}

	// Verify the signature.
	if cert.Issuer.CommonName != ca.ParsedCert.Subject.CommonName {
		return nil, Error{
			error:  fmt.Errorf("can't check a signature made by %q", cert.Issuer.CommonName),
			Reason: UnknownCA,
		}
	}
	if err = cert.CheckSignatureFrom(ca.ParsedCert); err != nil {
		return nil, Error{
			error:  err,
			Reason: SignatureCheckError,
		}
	}

	// Check the revocation list.
	switch revoked, err := ch.CRL.IsRevokedSN(c, cert.SerialNumber); {
	case err != nil:
		return nil, err
	case revoked:
		return nil, Error{
			error:  fmt.Errorf("certificate with SN %s has been revoked", cert.SerialNumber),
			Reason: CertificateRevoked,
		}
	}

	return ca, nil
}

// refetchCAPeriod returns for how long to cache the CA in memory by default.
//
// On dev server we cache for a very short duration to simplify local testing.
func refetchCAPeriod(c context.Context) time.Duration {
	if info.IsDevAppServer(c) {
		return 100 * time.Millisecond
	}
	return RefetchCAPeriod
}

// refetchCRLPeriod returns for how long to cache the CRL in memory by default.
//
// On dev server we cache for a very short duration to simplify local testing.
func refetchCRLPeriod(c context.Context) time.Duration {
	if info.IsDevAppServer(c) {
		return 100 * time.Millisecond
	}
	return RefetchCRLPeriod
}

// refetchCA is called lazily whenever we need to fetch the CA entity.
//
// If CA entity has disappeared since CertChecker was created, it returns empty
// certconfig.CA struct (that will be cached in ch.ca as usual). It acts as an
// indicator to GetCA to return NoSuchCA error, since returning a error here
// would just cause a retry of 'refetchCA' later, and returning (nil, nil) is
// forbidden (per lazyslot.Slot API).
func (ch *CertChecker) refetchCA(c context.Context) (*certconfig.CA, error) {
	ca := &certconfig.CA{CN: ch.CN}
	switch err := ds.Get(c, ca); {
	case err == ds.ErrNoSuchEntity:
		return &certconfig.CA{}, nil // GetCA knows that empty struct means "no such CA"
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}

	parsedConf, err := ca.ParseConfig()
	if err != nil {
		return nil, fmt.Errorf("can't parse stored config for %q - %s", ca.CN, err)
	}
	ca.ParsedConfig = parsedConf

	parsedCert, err := x509.ParseCertificate(ca.Cert)
	if err != nil {
		return nil, fmt.Errorf("can't parse stored cert for %q - %s", ca.CN, err)
	}
	ca.ParsedCert = parsedCert

	return ca, nil
}
