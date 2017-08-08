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

// Package machinetoken implements generation of LUCI machine tokens.
package machinetoken

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/tokensigning"
)

// tokenSigningContext is used to make sure machine token is not misused in
// place of some other token.
//
// See SigningContext in utils/tokensigning.Signer.
//
// TODO(vadimsh): Enable it. Requires some temporary hacks to accept old and
// new tokens at the same time.
const tokenSigningContext = ""

var maxUint64 = big.NewInt(0).SetUint64(math.MaxUint64)

// MintParams is passed to Mint.
type MintParams struct {
	// FQDN is a full name of a host to mint a token for.
	//
	// Must be in lowercase.
	FQDN string

	// Cert is the certificate used when authenticating the token requester.
	//
	// It's serial number will be put in the token.
	Cert *x509.Certificate

	// Config is a chunk of configuration related to the machine domain.
	//
	// It describes parameters for the token. Fetched from luci-config as part of
	// CA configuration.
	Config *admin.CertificateAuthorityConfig

	// Signer produces RSA-SHA256 signatures using a token server key.
	//
	// Usually it is using SignBytes GAE API.
	Signer signing.Signer
}

// Validate checks that token minting parameters are allowed.
func (p *MintParams) Validate() error {
	// Check FDQN.
	if p.FQDN != strings.ToLower(p.FQDN) {
		return fmt.Errorf("expecting FQDN in lowercase, got %q", p.FQDN)
	}
	chunks := strings.SplitN(p.FQDN, ".", 2)
	if len(chunks) != 2 {
		return fmt.Errorf("not a valid FQDN %q", p.FQDN)
	}
	domain := chunks[1] // e.g. "us-central1-a.c.project-id.internal"

	// Check DomainConfig for given domain.
	domainCfg := domainConfig(p.Config, domain)
	if domainCfg == nil {
		return fmt.Errorf("the domain %q is not whitelisted in the config", domain)
	}
	if domainCfg.MachineTokenLifetime <= 0 {
		return fmt.Errorf("machine tokens for machines in domain %q are not allowed", domain)
	}

	// Make sure cert serial number fits into uint64. We don't support negative or
	// giant SNs.
	sn := p.Cert.SerialNumber
	if sn.Sign() <= 0 || sn.Cmp(maxUint64) >= 0 {
		return fmt.Errorf("invalid certificate serial number: %s", sn)
	}
	return nil
}

// domainConfig returns DomainConfig (part of *.cfg file) for a given domain.
//
// It enumerates all domains specified in the config finding first domain that
// is equal to 'domain' or has it as a subdomain.
//
// Returns nil if requested domain is not represented in the config.
func domainConfig(cfg *admin.CertificateAuthorityConfig, domain string) *admin.DomainConfig {
	for _, domainCfg := range cfg.KnownDomains {
		for _, domainInCfg := range domainCfg.Domain {
			if domainInCfg == domain || strings.HasSuffix(domain, "."+domainInCfg) {
				return domainCfg
			}
		}
	}
	return nil
}

// Mint generates a new machine token proto, signs and serializes it.
//
// Returns its body as a proto, and as a signed base64-encoded final token.
func Mint(c context.Context, params *MintParams) (*tokenserver.MachineTokenBody, string, error) {
	if err := params.Validate(); err != nil {
		return nil, "", err
	}
	chunks := strings.SplitN(params.FQDN, ".", 2)
	if len(chunks) != 2 {
		panic("impossible") // checked in Validate already
	}
	cfg := domainConfig(params.Config, chunks[1])
	if cfg == nil {
		panic("impossible") // checked in Validate already
	}

	srvInfo, err := params.Signer.ServiceInfo(c)
	if err != nil {
		return nil, "", transient.Tag.Apply(err)
	}

	body := &tokenserver.MachineTokenBody{
		MachineFqdn: params.FQDN,
		IssuedBy:    srvInfo.ServiceAccountName,
		IssuedAt:    uint64(clock.Now(c).Unix()),
		Lifetime:    uint64(cfg.MachineTokenLifetime),
		CaId:        params.Config.UniqueId,
		CertSn:      params.Cert.SerialNumber.Uint64(), // already validated, fits uint64
	}
	signed, err := SignToken(c, params.Signer, body)
	if err != nil {
		return nil, "", err
	}
	return body, signed, nil
}

// SignToken signs and serializes the machine subtoken.
//
// It doesn't do any validation. Assumes the prepared subtoken is valid.
//
// Produces base64-encoded token or a transient error.
func SignToken(c context.Context, signer signing.Signer, body *tokenserver.MachineTokenBody) (string, error) {
	s := tokensigning.Signer{
		Signer:         signer,
		SigningContext: tokenSigningContext,
		Encoding:       base64.RawStdEncoding,
		Wrap: func(w *tokensigning.Unwrapped) proto.Message {
			return &tokenserver.MachineTokenEnvelope{
				TokenBody: w.Body,
				RsaSha256: w.RsaSHA256Sig,
				KeyId:     w.KeyID,
			}
		},
	}
	return s.SignToken(c, body)
}

// InspectToken returns information about the machine token.
//
// Inspection.Envelope is either nil or *tokenserver.MachineTokenEnvelope.
// Inspection.Body is either nil or *tokenserver.MachineTokenBody.
func InspectToken(c context.Context, certs tokensigning.CertificatesSupplier, tok string) (*tokensigning.Inspection, error) {
	i := tokensigning.Inspector{
		Encoding:       base64.RawStdEncoding,
		Certificates:   certs,
		SigningContext: tokenSigningContext,
		Envelope:       func() proto.Message { return &tokenserver.MachineTokenEnvelope{} },
		Body:           func() proto.Message { return &tokenserver.MachineTokenBody{} },
		Unwrap: func(e proto.Message) tokensigning.Unwrapped {
			env := e.(*tokenserver.MachineTokenEnvelope)
			return tokensigning.Unwrapped{
				Body:         env.TokenBody,
				RsaSHA256Sig: env.RsaSha256,
				KeyID:        env.KeyId,
			}
		},
		Lifespan: func(b proto.Message) tokensigning.Lifespan {
			body := b.(*tokenserver.MachineTokenBody)
			return tokensigning.Lifespan{
				NotBefore: time.Unix(int64(body.IssuedAt), 0),
				NotAfter:  time.Unix(int64(body.IssuedAt)+int64(body.Lifetime), 0),
			}
		},
	}
	return i.InspectToken(c, tok)
}
