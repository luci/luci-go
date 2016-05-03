// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package machinetoken implements generation of LUCI machine tokens.
package machinetoken

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"

	"github.com/luci/luci-go/common/api/tokenserver"
	"github.com/luci/luci-go/common/api/tokenserver/admin/v1"
)

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

	// ServiceHostname is hostname of the token server itself.
	//
	// It will become a part of machine_id field (embedded in the token).
	ServiceHostname string

	// Signer produces RSA-SHA256 signatures using a token server key.
	//
	// Usually it is SignBytes GAE API.
	Signer func(blob []byte) (keyID string, sig []byte, err error)
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
	host, domain := chunks[0], chunks[1]
	if strings.ContainsRune(host, '@') {
		return fmt.Errorf("forbidden character '@' in hostname %q", host)
	}

	// Check DomainConfig for given domain.
	domainCfg := domainConfig(p.Config, domain)
	if domainCfg == nil {
		return fmt.Errorf("the domain %q is not whitelisted in the config", domain)
	}
	if !domainCfg.AllowMachineTokens || domainCfg.MachineTokenLifetime <= 0 {
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

// domainConfig returns DomainConfig for a domain.
//
// Returns nil if there's no such config.
func domainConfig(cfg *admin.CertificateAuthorityConfig, domain string) *admin.DomainConfig {
	for _, domainCfg := range cfg.KnownDomains {
		for _, domainInCfg := range domainCfg.Domain {
			if domainInCfg == domain {
				return domainCfg
			}
		}
	}
	return nil
}

// Mint generates a new machine token.
//
// Returns its body as a proto, and as a signed base64-encoded final token.
func Mint(c context.Context, params MintParams) (*tokenserver.MachineTokenBody, string, error) {
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

	body := tokenserver.MachineTokenBody{
		MachineId: params.FQDN + "@" + params.ServiceHostname,
		IssuedAt:  uint64(clock.Now(c).Unix()),
		Lifetime:  uint64(cfg.MachineTokenLifetime),
		CaId:      params.Config.UniqueId,
		CertSn:    params.Cert.SerialNumber.Uint64(), // already validated, fits uint64
	}
	serializedBody, err := proto.Marshal(&body)
	if err != nil {
		return nil, "", err
	}

	keyID, signature, err := params.Signer(serializedBody)
	if err != nil {
		return nil, "", errors.WrapTransient(err)
	}

	tokenBinBlob, err := proto.Marshal(&tokenserver.MachineTokenEnvelope{
		TokenBody: serializedBody,
		KeyId:     keyID,
		RsaSha256: signature,
	})
	if err != nil {
		return nil, "", err
	}
	return &body, base64.RawStdEncoding.EncodeToString(tokenBinBlob), nil
}
