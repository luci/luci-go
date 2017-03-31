// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tokensigning

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth/signing"
)

// Inspector knows how to inspect tokens produced by Signer.
//
// It is used by Inspect<something>Token RPCs (available only to admins). It
// tries to return as much information as possible. In particular, it tries to
// deserialize the token body even if the signature is no longer valid. This is
// useful when debugging broken tokens.
//
// Since it is available only to admins, we assume the possibility of abuse is
// small.
type Inspector struct {
	// Certificates returns certs bundle used to validate the token signature.
	Certificates CertificatesSupplier

	// Encoding is base64 encoding to used for token (or RawURLEncoding if nil).
	Encoding *base64.Encoding

	// Envelope returns an empty message of same type as produced by signer.Wrap.
	Envelope func() proto.Message

	// Body returns an empty messages corresponding to the token body type.
	Body func() proto.Message

	// Unwrap extracts information from envelope proto message.
	//
	// It must set Body, RsaSHA256Sig and KeyID fields.
	Unwrap func(e proto.Message) Unwrapped

	// Lifespan extracts a lifespan from the deserialized body of the token.
	Lifespan func(e proto.Message) Lifespan
}

// CertificatesSupplier produces signing.PublicCertificates.
type CertificatesSupplier interface {
	// Certificates returns certs bundle used to validate the token signature.
	Certificates(c context.Context) (*signing.PublicCertificates, error)
}

// Inspection is the result of token inspection.
type Inspection struct {
	Signed           bool          // true if the token is structurally valid and signed
	NonExpired       bool          // true if the token hasn't expire yet (may be bogus for unsigned tokens)
	InvalidityReason string        // human readable reason why the token is invalid or "" if it is valid
	Envelope         proto.Message // deserialized token envelope
	Body             proto.Message // deserialized token body
}

// InspectToken extracts as much information as possible from the token.
//
// Returns errors only if the inspection operation itself fails (i.e we can't
// determine whether the token valid or not). If the given token is invalid,
// returns Inspection object with details and nil error.
func (i *Inspector) InspectToken(c context.Context, tok string) (*Inspection, error) {
	res := &Inspection{}

	enc := i.Encoding
	if enc == nil {
		enc = base64.RawURLEncoding
	}

	// Byte blob with serialized envelope.
	blob, err := enc.DecodeString(tok)
	if err != nil {
		res.InvalidityReason = fmt.Sprintf("not base64 - %s", err)
		return res, nil
	}

	// Deserialize the envelope into a proto message.
	env := i.Envelope()
	if err = proto.Unmarshal(blob, env); err != nil {
		res.InvalidityReason = fmt.Sprintf("can't unmarshal the envelope - %s", err)
		return res, nil
	}
	res.Envelope = env

	// Convert opaque proto message into a struct we can work with.
	unwrapped := i.Unwrap(res.Envelope)

	// Try to deserialize the body, if possible.
	body := i.Body()
	if err = proto.Unmarshal(unwrapped.Body, body); err != nil {
		res.InvalidityReason = fmt.Sprintf("can't unmarshal the token body - %s", err)
		return res, nil
	}
	res.Body = body

	var reasons []string

	if reason := i.checkLifetime(c, body); reason != "" {
		reasons = append(reasons, reason)
	} else {
		res.NonExpired = true
	}

	switch reason, err := i.checkSignature(c, &unwrapped); {
	case err != nil:
		return nil, err
	case reason != "":
		reasons = append(reasons, reason)
	default:
		res.Signed = true
	}

	res.InvalidityReason = strings.Join(reasons, "; ")
	return res, nil
}

// checkLifetime checks that token has not expired yet.
//
// Returns "" if it hasn't expire yet, or an invalidity reason if it has.
func (i *Inspector) checkLifetime(c context.Context, body proto.Message) string {
	lifespan := i.Lifespan(body)
	now := clock.Now(c)
	switch {
	case lifespan.NotAfter == lifespan.NotBefore:
		return "can't extract the token lifespan"
	case now.Before(lifespan.NotBefore):
		return "not active yet"
	case now.After(lifespan.NotAfter):
		return "expired"
	default:
		return ""
	}
}

// checkSignature verifies the signature of the token.
//
// Returns "" if the signature is correct, or an invalidity reason if it is not.
func (i *Inspector) checkSignature(c context.Context, unwrapped *Unwrapped) (string, error) {
	certsBundle, err := i.Certificates.Certificates(c)
	if err != nil {
		return "", errors.WrapTransient(err)
	}
	cert, err := certsBundle.CertificateForKey(unwrapped.KeyID)
	if err != nil {
		return fmt.Sprintf("invalid signing key - %s", err), nil
	}
	err = cert.CheckSignature(x509.SHA256WithRSA, unwrapped.Body, unwrapped.RsaSHA256Sig)
	if err != nil {
		return fmt.Sprintf("bad signature - %s", err), nil
	}
	return "", nil
}
