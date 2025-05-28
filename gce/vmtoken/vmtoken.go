// Copyright 2019 The LUCI Authors.
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

// Package vmtoken implements parsing and verification of signed GCE VM metadata
// tokens.
//
// See https://cloud.google.com/compute/docs/instances/verifying-instance-identity
//
// Intended to be used from a server environment (e.g. from a GAE), since it
// depends on a bunch of luci/server packages that require a properly configured
// context.
package vmtoken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/warmup"
)

// Header is the name of the HTTP header where the GCE VM metadata token is
// expected.
const Header = "X-Luci-Gce-Vm-Token"

// Payload is extracted from a verified GCE VM metadata token.
//
// It identifies a VM that produced the token and the target audience for
// the token (as it was supplied to the GCE metadata endpoint via 'audience'
// request parameter when generating the token).
type Payload struct {
	Project  string // GCE project name, e.g. "my-bots" or "domain.com:my-bots"
	Zone     string // GCE zone name where the VM is, e.g. "us-central1-b"
	Instance string // VM instance name, e.g. "my-instance-1"
	Audience string // 'aud' field inside the token, usually the server URL
}

// Verify parses a GCE VM metadata token, verifies its signature and expiration
// time, and extracts interesting parts of it into Payload struct.
//
// Does NOT verify the audience field. This is responsibility of the caller.
//
// The token is in JWT form (three dot-separated base64-encoded strings). It is
// expected to be signed by Google OAuth2 backends using RS256 algo.
func Verify(c context.Context, jwt string) (*Payload, error) {
	// Grab root Google OAuth2 keys to verify JWT signature. They are most likely
	// already cached in the process memory.
	certs, err := signing.FetchGoogleOAuth2Certificates(c)
	if err != nil {
		return nil, err
	}
	return verifyImpl(c, jwt, certs)
}

// signatureChecker is used to mock signing.PublicCertificates in tests.
type signatureChecker interface {
	CheckSignature(key string, signed, signature []byte) error
}

func verifyImpl(c context.Context, jwt string, certs signatureChecker) (*Payload, error) {
	chunks := strings.Split(jwt, ".")
	if len(chunks) != 3 {
		return nil, errors.New("bad JWT: expected 3 components separated by '.'")
	}

	// Check the header, grab the key ID from it.
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := unmarshalB64JSON(chunks[0], &hdr); err != nil {
		return nil, errors.Fmt("bad JWT header: %w", err)
	}
	if hdr.Alg != "RS256" {
		return nil, errors.Fmt("bad JWT: only RS256 alg is supported, not %q", hdr.Alg)
	}
	if hdr.Kid == "" {
		return nil, errors.New("bad JWT: missing the signing key ID in the header")
	}

	// Need a raw binary blob with the signature to verify it.
	sig, err := base64.RawURLEncoding.DecodeString(chunks[2])
	if err != nil {
		return nil, errors.Fmt("bad JWT: can't base64 decode the signature: %w", err)
	}

	// Check that "b64(hdr).b64(payload)" part of the token matches the signature.
	// If it does, we know the token was created by Google.
	if err = certs.CheckSignature(hdr.Kid, []byte(chunks[0]+"."+chunks[1]), sig); err != nil {
		return nil, errors.Fmt("bad JWT: bad signature: %w", err)
	}

	// Decode the payload. There should be no errors here generally, the encoded
	// payload is signed and the signature was already verified. Note that for the
	// sake of completeness and documentation we decode all fields usually present
	// in the token, even though we use only subset of them below.
	var payload struct {
		Aud           string `json:"aud"`            // audience
		Azp           string `json:"azp"`            // authorized party (GCE VM service account ID)
		Email         string `json:"email"`          // GCE VM service account email
		EmailVerified bool   `json:"email_verified"` // always true
		Exp           int64  `json:"exp"`            // "expiry", as unix timestamp
		Iat           int64  `json:"iat"`            // "issued at", as unix timestamp
		Iss           string `json:"iss"`            // issuer name
		Sub           string `json:"sub"`            // subject (GCE VM service account ID)
		Google        struct {
			ComputeEngine struct {
				InstanceCreationTimestamp int64  `json:"instance_creation_timestamp"`
				InstanceID                string `json:"instance_id"`
				InstanceName              string `json:"instance_name"`
				ProjectID                 string `json:"project_id"`
				ProjectNumber             int64  `json:"project_number"`
				Zone                      string `json:"zone"`
			} `json:"compute_engine"`
		} `json:"google"`
	}
	if err = unmarshalB64JSON(chunks[1], &payload); err != nil {
		return nil, errors.Fmt("bad JWT payload: %w", err)
	}

	// Tokens can either be in "full" or "standard" format. We want "full", since
	// "standard" doesn't have details about the VM.
	if payload.Google.ComputeEngine.ProjectID == "" {
		return nil, errors.New("no google.compute_engine in the GCE VM token, use 'full' format")
	}

	// Check token's "issued at" and "expiry" claims. Allow some leeway for clock
	// discrepancy between us and Google OAuth2 backend.
	const allowedDriftSec = 30
	switch now := clock.Now(c).Unix(); {
	case now < payload.Iat-allowedDriftSec:
		return nil, errors.Fmt("bad JWT: too early (now %d < iat %d)", now, payload.Iat)
	case now > payload.Exp+allowedDriftSec:
		return nil, errors.Fmt("bad JWT: expired (now %d > exp %d)", now, payload.Exp)
	}

	// The caller is supposed to check 'aud' claim to finish the verification.
	return &Payload{
		Project:  payload.Google.ComputeEngine.ProjectID,
		Zone:     payload.Google.ComputeEngine.Zone,
		Instance: payload.Google.ComputeEngine.InstanceName,
		Audience: payload.Aud,
	}, nil
}

func unmarshalB64JSON(blob string, out any) error {
	raw, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		return errors.Fmt("not base64: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.Fmt("not JSON: %w", err)
	}
	return nil
}

// pldKey is the key to a *Payload in the context.
var pldKey = "pld"

// withPayload returns a new context with the given *Payload installed.
func withPayload(c context.Context, p *Payload) context.Context {
	return context.WithValue(c, &pldKey, p)
}

// getPayload returns the *Payload installed in the current context. May be nil.
func getPayload(c context.Context) *Payload {
	p, _ := c.Value(&pldKey).(*Payload)
	return p
}

// Clear returns a new context without a GCE VM metadata token installed.
func Clear(c context.Context) context.Context {
	return context.WithValue(c, &pldKey, nil)
}

// Has returns whether the current context contains a valid GCE VM metadata
// token.
func Has(c context.Context) bool {
	return getPayload(c) != nil
}

// Hostname returns the hostname of the VM stored in the current context.
func Hostname(c context.Context) string {
	p := getPayload(c)
	if p == nil {
		return ""
	}
	return p.Instance
}

// CurrentIdentity returns the identity of the VM stored in the current context.
func CurrentIdentity(c context.Context) string {
	p := getPayload(c)
	if p == nil {
		return "gce:anonymous"
	}
	// GCE hostnames must be unique per project, so <instance, project> suffices.
	return fmt.Sprintf("gce:%s:%s", p.Instance, p.Project)
}

// Matches returns whether the current context contains a GCE VM metadata
// token matching the given identity.
func Matches(c context.Context, host, zone, proj string) bool {
	p := getPayload(c)
	if p == nil {
		return false
	}
	logging.Debugf(c, "expecting VM token from %q in %q in %q", host, zone, proj)
	return p.Instance == host && p.Zone == zone && p.Project == proj
}

// Middleware embeds a Payload in the context if the request contains a GCE VM
// metadata token.
func Middleware(c *router.Context, next router.Handler) {
	if tok := c.Request.Header.Get(Header); tok != "" {
		// TODO(smut): Support requests to other modules, versions.
		aud := "https://" + info.DefaultVersionHostname(c.Request.Context())
		logging.Debugf(c.Request.Context(), "expecting VM token for: %s", aud)
		switch p, err := Verify(c.Request.Context(), tok); {
		case transient.Tag.In(err):
			logging.WithError(err).Errorf(c.Request.Context(), "transient error verifying VM token")
			http.Error(c.Writer, "error: failed to verify VM token", http.StatusInternalServerError)
			return
		case err != nil:
			logging.WithError(err).Errorf(c.Request.Context(), "invalid VM token")
			http.Error(c.Writer, "error: invalid VM token", http.StatusUnauthorized)
			return
		case p.Audience != aud:
			logging.Errorf(c.Request.Context(), "received VM token intended for: %s", p.Audience)
			http.Error(c.Writer, "error: VM token audience mismatch", http.StatusUnauthorized)
			return
		default:
			logging.Debugf(c.Request.Context(), "received VM token from %q in %q in %q for: %s", p.Instance, p.Zone, p.Project, p.Audience)
			c.Request = c.Request.WithContext(withPayload(c.Request.Context(), p))
		}
	}
	next(c)
}

func init() {
	warmup.Register("gce/vmtoken", func(c context.Context) error {
		_, err := signing.FetchGoogleOAuth2Certificates(c)
		return err
	})
}
