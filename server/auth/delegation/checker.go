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

package delegation

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/internal/tracing"
	"go.chromium.org/luci/server/auth/signing"
)

const (
	// maxTokenSize is upper bound for expected size of a token (after base64
	// decoding). Larger tokens will be ignored right away.
	maxTokenSize = 8 * 1024

	// allowedClockDriftSec is how much clock difference we accept, in seconds.
	allowedClockDriftSec = int64(30)
)

var (
	// ErrMalformedDelegationToken is returned when delegation token cannot be
	// deserialized.
	ErrMalformedDelegationToken = errors.New("auth: malformed delegation token")

	// ErrUnsignedDelegationToken is returned if token's signature cannot be
	// verified.
	ErrUnsignedDelegationToken = errors.New("auth: unsigned delegation token")

	// ErrForbiddenDelegationToken is returned if token is structurally correct,
	// but some of its constraints prevents it from being used. For example, it is
	// already expired or it was minted for some other services, etc. See logs for
	// details.
	ErrForbiddenDelegationToken = errors.New("auth: forbidden delegation token")
)

// CertificatesProvider is used by 'CheckToken', it is implemented by authdb.DB.
//
// It returns certificates of services trusted to sign tokens.
type CertificatesProvider interface {
	// GetCertificates returns a bundle with certificates of a trusted signer.
	//
	// Returns (nil, nil) if the given signer is not trusted.
	//
	// Returns errors (usually transient) if the bundle can't be fetched.
	GetCertificates(ctx context.Context, id identity.Identity) (*signing.PublicCertificates, error)
}

// GroupsChecker is accepted by 'CheckToken', it is implemented by authdb.DB.
type GroupsChecker interface {
	// IsMember returns true if the given identity belongs to any of the groups.
	//
	// Unknown groups are considered empty. May return errors if underlying
	// datastore has issues.
	IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error)
}

// CheckTokenParams is passed to CheckToken.
type CheckTokenParams struct {
	Token                string               // the delegation token to check
	PeerID               identity.Identity    // identity of the caller, as extracted from its credentials
	CertificatesProvider CertificatesProvider // returns certificates with trusted keys
	GroupsChecker        GroupsChecker        // knows how to do group lookups
	OwnServiceIdentity   identity.Identity    // identity of the current service
}

// CheckToken verifies validity of a delegation token.
//
// If the token is valid, it returns the delegated identity (embedded in the
// token).
//
// May return transient errors.
func CheckToken(ctx context.Context, params CheckTokenParams) (_ identity.Identity, err error) {
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth/delegation.CheckToken")
	defer func() { tracing.End(span, err) }()

	// base64-encoded token -> DelegationToken proto (with signed serialized
	// subtoken).
	tok, err := deserializeToken(params.Token)
	if err != nil {
		logging.Warningf(ctx, "auth: Failed to deserialize delegation token - %s", err)
		return "", ErrMalformedDelegationToken
	}

	// Signed serialized subtoken -> Subtoken proto.
	subtoken, err := unsealToken(ctx, tok, params.CertificatesProvider)
	if err != nil {
		if transient.Tag.In(err) {
			logging.Warningf(ctx, "auth: Transient error when checking delegation token signature - %s", err)
			return "", err
		}
		logging.Warningf(ctx, "auth: Failed to check delegation token signature - %s", err)
		return "", ErrUnsignedDelegationToken
	}

	// Validate all constrains encoded in the token and derive the delegated
	// identity.
	return checkSubtoken(ctx, subtoken, &params)
}

// deserializeToken deserializes DelegationToken proto message.
func deserializeToken(token string) (*messages.DelegationToken, error) {
	blob, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	if len(blob) > maxTokenSize {
		return nil, fmt.Errorf("the delegation token is too big (%d bytes)", len(blob))
	}
	tok := &messages.DelegationToken{}
	if err = proto.Unmarshal(blob, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// unsealToken verifies token's signature and deserializes the subtoken.
//
// May return transient errors.
func unsealToken(ctx context.Context, tok *messages.DelegationToken, certsProvider CertificatesProvider) (*messages.Subtoken, error) {
	// Grab the public keys of the service that signed the token, if we trust it.
	signerID, err := identity.MakeIdentity(tok.SignerId)
	if err != nil {
		return nil, fmt.Errorf("bad signer_id %q - %s", tok.SignerId, err)
	}
	certs, err := certsProvider.GetCertificates(ctx, signerID)
	switch {
	case err != nil:
		return nil, fmt.Errorf("failed to grab certificates of %q - %s", tok.SignerId, err)
	case certs == nil:
		return nil, fmt.Errorf("the signer %q is not trusted", tok.SignerId)
	}

	// Check the signature on the token.
	err = certs.CheckSignature(tok.SigningKeyId, tok.SerializedSubtoken, tok.Pkcs1Sha256Sig)
	if err != nil {
		return nil, err
	}

	// The signature is correct! Deserialize the subtoken.
	msg := &messages.Subtoken{}
	if err = proto.Unmarshal(tok.SerializedSubtoken, msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// checkSubtoken validates the delegation subtoken.
//
// It extracts and returns original delegated_identity.
func checkSubtoken(ctx context.Context, subtoken *messages.Subtoken, params *CheckTokenParams) (identity.Identity, error) {
	if subtoken.Kind != messages.Subtoken_BEARER_DELEGATION_TOKEN {
		logging.Warningf(ctx, "auth: Invalid delegation token kind - %s", subtoken.Kind)
		return "", ErrForbiddenDelegationToken
	}

	// Do fast checks before heavy ones.
	now := clock.Now(ctx).Unix()
	if err := checkSubtokenExpiration(subtoken, now); err != nil {
		logging.Warningf(ctx, "auth: Bad delegation token expiration - %s", err)
		return "", ErrForbiddenDelegationToken
	}
	if err := checkSubtokenServices(subtoken, params.OwnServiceIdentity); err != nil {
		logging.Warningf(ctx, "auth: Forbidden delegation token - %s", err)
		return "", ErrForbiddenDelegationToken
	}

	// Do the audience check (may use group lookups).
	if err := checkSubtokenAudience(ctx, subtoken, params.PeerID, params.GroupsChecker); err != nil {
		if transient.Tag.In(err) {
			logging.Warningf(ctx, "auth: Transient error when checking delegation token audience - %s", err)
			return "", err
		}
		logging.Warningf(ctx, "auth: Bad delegation token audience - %s", err)
		return "", ErrForbiddenDelegationToken
	}

	// Grab delegated identity.
	ident, err := identity.MakeIdentity(subtoken.DelegatedIdentity)
	if err != nil {
		logging.Warningf(ctx, "auth: Invalid delegated_identity in the delegation token - %s", err)
		return "", ErrMalformedDelegationToken
	}

	return ident, nil
}

// checkSubtokenExpiration checks 'CreationTime' and 'ValidityDuration' fields.
func checkSubtokenExpiration(t *messages.Subtoken, now int64) error {
	if t.CreationTime <= 0 {
		return fmt.Errorf("invalid 'creation_time' field: %d", t.CreationTime)
	}
	dur := int64(t.ValidityDuration)
	if dur <= 0 {
		return fmt.Errorf("invalid validity_duration: %d", dur)
	}
	if t.CreationTime >= now+allowedClockDriftSec {
		return fmt.Errorf("token is not active yet (created at %d)", t.CreationTime)
	}
	if t.CreationTime+dur < now {
		return fmt.Errorf("token has expired %d sec ago", now-(t.CreationTime+dur))
	}
	return nil
}

// checkSubtokenServices makes sure the token is usable by the current service.
func checkSubtokenServices(t *messages.Subtoken, serviceID identity.Identity) error {
	// Empty services field is not allowed.
	if len(t.Services) == 0 {
		return fmt.Errorf("the token's services list is empty")
	}
	// Else, make sure we are in the 'services' list or it contains '*'.
	for _, allowed := range t.Services {
		if allowed == "*" || allowed == string(serviceID) {
			return nil
		}
	}
	return fmt.Errorf("token is not intended for %s", serviceID)
}

// checkSubtokenAudience makes sure the token is intended for use by given
// identity.
//
// May return transient errors.
func checkSubtokenAudience(ctx context.Context, t *messages.Subtoken, ident identity.Identity, checker GroupsChecker) error {
	// Empty audience field is not allowed.
	if len(t.Audience) == 0 {
		return fmt.Errorf("the token's audience list is empty")
	}
	// Try to find a direct hit first, to avoid calling expensive group lookups.
	// Collect the groups along the way for the check below.
	groups := make([]string, 0, len(t.Audience))
	for _, aud := range t.Audience {
		if aud == "*" || aud == string(ident) {
			return nil
		}
		if strings.HasPrefix(aud, "group:") {
			groups = append(groups, strings.TrimPrefix(aud, "group:"))
		}
	}
	// Search through groups now.
	switch ok, err := checker.IsMember(ctx, ident, groups); {
	case err != nil:
		return err // transient error during group lookup
	case ok:
		return nil // success, 'ident' is in the target audience
	}
	return fmt.Errorf("%s is not allowed to use the token", ident)
}
