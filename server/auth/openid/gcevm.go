// Copyright 2022 The LUCI Authors.
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

package openid

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
)

// GoogleComputeAuthMethod implements auth.Method by checking a header which is
// expected to have an OpenID Connect ID token generated via GCE VM identity
// metadata endpoint.
//
// Such tokens identify a particular VM via `google.compute_engine` claim. ID
// tokens without that claim (even if they pass the signature checks) are
// rejected.
//
// The authenticated identity has form "bot:<instance-name>@gce.<project>", but
// instead of parsing it, better to use GetGoogleComputeTokenInfo to get the
// information extracted from the token in a structured form.
type GoogleComputeAuthMethod struct {
	// Header is a HTTP header to read the token from. Required.
	Header string
	// AudienceCheck is a callback to use to check tokens audience. Required.
	AudienceCheck func(ctx context.Context, r auth.RequestMetadata, aud string) (valid bool, err error)

	// certs are used in tests in place of Google certificates.
	certs *signing.PublicCertificates
}

// Make sure all extra interfaces are implemented.
var _ interface {
	auth.Method
	auth.Warmable
} = (*GoogleComputeAuthMethod)(nil)

// GoogleComputeTokenInfo contains information extracted from the GCM VM token.
type GoogleComputeTokenInfo struct {
	// Audience is the audience in the token as it was checked by AudienceCheck.
	Audience string
	// ServiceAccount is the service account email the GCE VM runs under.
	ServiceAccount string
	// Instance is a GCE VM instance name asserted in the token.
	Instance string
	// Zone is the GCE zone with the VM.
	Zone string
	// Project is a GCP project name the VM belong to.
	Project string
}

// GetGoogleComputeTokenInfo returns GCE VM info as asserted by the VM token.
//
// Works only from within a request handler and only if the call was
// authenticated via a GCE VM token. In all other cases (anonymous calls, calls
// authenticated via some other mechanism, etc.) returns nil.
func GetGoogleComputeTokenInfo(ctx context.Context) *GoogleComputeTokenInfo {
	info, _ := auth.CurrentUser(ctx).Extra.(*GoogleComputeTokenInfo)
	return info
}

// Authenticate extracts user information from the incoming request.
//
// It returns:
//   - (*User, nil, nil) on success.
//   - (nil, nil, nil) if the method is not applicable.
//   - (nil, nil, error) if the method is applicable, but credentials are bad.
func (m *GoogleComputeAuthMethod) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header(m.Header), "Bearer "))
	if token == "" {
		return nil, nil, nil // skip this auth method
	}

	// Grab root Google OAuth2 keys to verify JWT signature. They are most likely
	// already cached in the process memory.
	certs := m.certs
	if certs == nil {
		var err error
		if certs, err = signing.FetchGoogleOAuth2Certificates(ctx); err != nil {
			return nil, nil, err
		}
	}

	// Verify and deserialize the token. GCE VM tokens are always issued by
	// accounts.google.com.
	verifiedToken, err := VerifyIDToken(ctx, token, certs, "https://accounts.google.com")
	if err != nil {
		return nil, nil, err
	}

	// Tokens can either be in "full" or "standard" format. We want "full", since
	// "standard" doesn't have details about the VM.
	if verifiedToken.Google.ComputeEngine.ProjectID == "" {
		return nil, nil, errors.Reason("no google.compute_engine in the GCE VM token, use 'full' format").Err()
	}

	// Convert "<realm>:<project>" to "<project>.<realm>" for "bot:..." string.
	domain := verifiedToken.Google.ComputeEngine.ProjectID
	if chunks := strings.SplitN(domain, ":", 2); len(chunks) == 2 {
		domain = fmt.Sprintf("%s.%s", chunks[1], chunks[0])
	}

	// Generate some "bot" identity just to have something representative in the
	// context for e.g. logs. This also verifies there are no funky characters
	// in the instance and project names. Full information about the token will be
	// exposed via GoogleComputeTokenInfo in auth.User.Extra.
	ident, err := identity.MakeIdentity(fmt.Sprintf("bot:%s@gce.%s",
		verifiedToken.Google.ComputeEngine.InstanceName, domain))
	if err != nil {
		return nil, nil, err
	}

	// Check the audience in the token.
	if m.AudienceCheck == nil {
		return nil, nil, errors.Reason("GoogleComputeAuthMethod has no AudienceCheck").Err()
	}
	switch valid, err := m.AudienceCheck(ctx, r, verifiedToken.Aud); {
	case err != nil:
		return nil, nil, err
	case !valid:
		logging.Errorf(ctx, "openid: GCE VM token from %s has unrecognized audience %q", ident, verifiedToken.Aud)
		return nil, nil, auth.ErrBadAudience
	}

	// Success.
	return &auth.User{
		Identity: ident,
		Extra: &GoogleComputeTokenInfo{
			Audience:       verifiedToken.Aud,
			ServiceAccount: verifiedToken.Email,
			Instance:       verifiedToken.Google.ComputeEngine.InstanceName,
			Zone:           verifiedToken.Google.ComputeEngine.Zone,
			Project:        verifiedToken.Google.ComputeEngine.ProjectID,
		},
	}, nil, nil
}

// Warmup prepares local caches. It's optional.
//
// Implements auth.Warmable.
func (m *GoogleComputeAuthMethod) Warmup(ctx context.Context) error {
	_, err := signing.FetchGoogleOAuth2Certificates(ctx)
	return err
}
