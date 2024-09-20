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

// Package machine implements authentication based on LUCI machine tokens.
package machine

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"

	tokenserverpb "go.chromium.org/luci/tokenserver/api"
)

const (
	// MachineTokenHeader is an HTTP header that carries the machine token.
	MachineTokenHeader = "X-Luci-Machine-Token"

	// TokenServersGroup is name of a group with trusted token servers.
	//
	// This group should contain service account emails of token servers we trust.
	TokenServersGroup = "auth-token-servers"

	// allowedClockDrift is how much clock difference we tolerate.
	allowedClockDrift = 10 * time.Second
)

var (
	// ErrBadToken is returned if the supplied machine token is not valid.
	//
	// See app logs for more details.
	ErrBadToken = errors.New("bad machine token")
)

// MachineTokenAuthMethod implements auth.Method by verifying machine tokens.
//
// It looks at X-Luci-Machine-Token header and verifies that it contains a valid
// non-expired machine token issued by some trusted token server instance.
//
// A list of trusted token servers is specified in 'auth-token-servers' group.
//
// If the token is valid, the request will be authenticated as coming from
// 'bot:<machine_fqdn>', where <machine_fqdn> is extracted from the token. It is
// lowercase FQDN of a machine (as specified in the certificate used to mint the
// token).
//
// Full information about the token can be obtained via GetMachineTokenInfo.
type MachineTokenAuthMethod struct {
	// certsFetcher is mocked in unit tests.
	//
	// In prod it is based on signing.FetchCertificatesForServiceAccount.
	certsFetcher func(ctx context.Context, email string) (*signing.PublicCertificates, error)
}

// MachineTokenInfo contains information extracted from the LUCI machine token.
type MachineTokenInfo struct {
	// FQDN is machine's FQDN as asserted by the token.
	//
	// It is extracted from the machine certificate use to obtain the machine
	// token.
	FQDN string
	// CA identifies the Certificate Authority that signed the machine cert.
	//
	// It is an integer ID of the certificate authority as specified in the
	// LUCI Token Server configs.
	CA int64
	// CertSN is the machine certificate serial number used to get the machine
	// token.
	CertSN []byte
}

// GetMachineTokenInfo returns the information extracted from the machine token.
//
// Works only from within a request handler and only if the call was
// authenticated via a LUCI machine token. In all other cases (anonymous calls,
// calls authenticated via some other mechanism, etc.) returns nil.
func GetMachineTokenInfo(ctx context.Context) *MachineTokenInfo {
	info, _ := auth.CurrentUser(ctx).Extra.(*MachineTokenInfo)
	return info
}

// Authenticate extracts peer's identity from the incoming request.
//
// It logs detailed errors in log, but returns only generic "bad credential"
// error to the caller, to avoid leaking unnecessary information.
func (m *MachineTokenAuthMethod) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	token := r.Header(MachineTokenHeader)
	if token == "" {
		return nil, nil, nil // no token -> the auth method is not applicable
	}

	// Deserialize both envelope and the body.
	envelope, body, err := deserialize(token)
	if err != nil {
		logTokenError(ctx, body, "Failed to deserialize the token: %s", err)
		return nil, nil, ErrBadToken
	}

	// Construct an identity of a token server that signed the token to check that
	// it belongs to "auth-token-servers" group.
	signerServiceAccount, err := identity.MakeIdentity("user:" + body.IssuedBy)
	if err != nil {
		logTokenError(ctx, body, "Bad issued_by field %q: %s", body.IssuedBy, err)
		return nil, nil, ErrBadToken
	}

	// Reject tokens from unknown token servers right away.
	db, err := auth.GetDB(ctx)
	if err != nil {
		return nil, nil, transient.Tag.Apply(err)
	}
	ok, err := db.IsMember(ctx, signerServiceAccount, []string{TokenServersGroup})
	if err != nil {
		return nil, nil, transient.Tag.Apply(err)
	}
	if !ok {
		logTokenError(ctx, body, "Unknown token issuer %q", body.IssuedBy)
		return nil, nil, ErrBadToken
	}

	// Check the expiration time before doing any heavier checks.
	if err = checkExpiration(body, clock.Now(ctx)); err != nil {
		logTokenError(ctx, body, "Token has expired or not yet valid: %s", err)
		return nil, nil, ErrBadToken
	}

	// Check the token was actually signed by the server.
	if err = m.checkSignature(ctx, body.IssuedBy, envelope); err != nil {
		if transient.Tag.In(err) {
			return nil, nil, err
		}
		logTokenError(ctx, body, "Bad signature: %s", err)
		return nil, nil, ErrBadToken
	}

	// The token is valid. Construct the bot identity.
	botIdent, err := identity.MakeIdentity("bot:" + body.MachineFqdn)
	if err != nil {
		logTokenError(ctx, body, "Bad machine_fqdn %q: %s", body.MachineFqdn, err)
		return nil, nil, ErrBadToken
	}
	return &auth.User{
		Identity: botIdent,
		Extra: &MachineTokenInfo{
			FQDN:   body.MachineFqdn,
			CA:     body.CaId,
			CertSN: body.CertSn,
		},
	}, nil, nil
}

// logTokenError adds a warning-level log entry with details about the request.
func logTokenError(ctx context.Context, tok *tokenserverpb.MachineTokenBody, msg string, args ...any) {
	fields := logging.Fields{}
	if tok != nil {
		// Note that if token wasn't properly signed, these fields may contain
		// garbage.
		fields["machineFqdn"] = tok.MachineFqdn
		fields["issuedBy"] = tok.IssuedBy
		fields["issuedAt"] = fmt.Sprintf("%d", tok.IssuedAt)
		fields["lifetime"] = fmt.Sprintf("%d", tok.Lifetime)
		fields["caId"] = fmt.Sprintf("%d", tok.CaId)
		fields["certSn"] = hex.EncodeToString(tok.CertSn)
	}
	fields.Warningf(ctx, msg, args...)
}

// deserialize parses MachineTokenEnvelope and MachineTokenBody.
func deserialize(token string) (*tokenserverpb.MachineTokenEnvelope, *tokenserverpb.MachineTokenBody, error) {
	tokenBinBlob, err := base64.RawStdEncoding.DecodeString(token)
	if err != nil {
		return nil, nil, err
	}
	envelope := &tokenserverpb.MachineTokenEnvelope{}
	if err := proto.Unmarshal(tokenBinBlob, envelope); err != nil {
		return nil, nil, err
	}
	body := &tokenserverpb.MachineTokenBody{}
	if err := proto.Unmarshal(envelope.TokenBody, body); err != nil {
		return envelope, nil, err
	}
	return envelope, body, nil
}

// checkExpiration returns nil if the token is non-expired yet.
//
// Allows some clock drift, see allowedClockDrift.
func checkExpiration(body *tokenserverpb.MachineTokenBody, now time.Time) error {
	notBefore := time.Unix(int64(body.IssuedAt), 0)
	notAfter := notBefore.Add(time.Duration(body.Lifetime) * time.Second)
	if now.Before(notBefore.Add(-allowedClockDrift)) {
		diff := notBefore.Sub(now)
		return fmt.Errorf("token is not valid yet, will be valid in %s", diff)
	}
	if now.After(notAfter.Add(allowedClockDrift)) {
		diff := now.Sub(notAfter)
		return fmt.Errorf("token expired %s ago", diff)
	}
	return nil
}

// checkSignature verifies the token signature.
func (m *MachineTokenAuthMethod) checkSignature(ctx context.Context, signerEmail string, envelope *tokenserverpb.MachineTokenEnvelope) error {
	// Note that FetchCertificatesForServiceAccount implements caching inside.
	fetcher := m.certsFetcher
	if fetcher == nil {
		fetcher = signing.FetchCertificatesForServiceAccount
	}
	certs, err := fetcher(ctx, signerEmail)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	return certs.CheckSignature(envelope.KeyId, envelope.TokenBody, envelope.RsaSha256)
}
