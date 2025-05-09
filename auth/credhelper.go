// Copyright 2025 The LUCI Authors.
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

package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/credhelperpb"
	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"
)

// CheckCredentialHelperConfig checks the config has all necessary fields set.
func CheckCredentialHelperConfig(cfg *credhelperpb.Config) error {
	if cfg.Exec == "" {
		return errors.Fmt("missing required \"exec\" field")
	}
	if _, err := externalHandler(cfg.Protocol); err != nil {
		return err
	}
	return nil
}

type credHelperTokenProvider struct {
	cfg     *credhelperpb.Config
	handler func(stdout []byte) (*oauth2.Token, error)
}

// newCredHelperTokenProvider makes an internal.TokenProvider that calls
// the credential helper binary.
func newCredHelperTokenProvider(cfg *credhelperpb.Config) (internal.TokenProvider, error) {
	if err := CheckCredentialHelperConfig(cfg); err != nil {
		return nil, errors.Fmt("bad credential helper config: %w", err)
	}
	exe, err := exec.LookPath(cfg.Exec)
	if err != nil {
		return nil, errors.Fmt("bad credential helper path %q: %w", cfg.Exec, err)
	}
	handler, _ := externalHandler(cfg.Protocol)
	return &credHelperTokenProvider{
		cfg: &credhelperpb.Config{
			Protocol: cfg.Protocol,
			Exec:     exe,
			Args:     cfg.Args,
		},
		handler: handler,
	}, nil
}

// RequiresInteraction implements internal.TokenProvider.
func (p *credHelperTokenProvider) RequiresInteraction() bool {
	return false
}

// RequiresWarmup implements internal.TokenProvider.
func (p *credHelperTokenProvider) RequiresWarmup() bool {
	return true
}

// MemoryCacheOnly implements internal.TokenProvider.
func (p *credHelperTokenProvider) MemoryCacheOnly() bool {
	return true
}

// Email implements internal.TokenProvider.
func (p *credHelperTokenProvider) Email() (string, error) {
	return "", internal.ErrUnimplementedEmail
}

// CacheKey implements internal.TokenProvider.
func (p *credHelperTokenProvider) CacheKey(ctx context.Context) (*internal.CacheKey, error) {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%q\n", p.cfg.Exec)
	_, _ = fmt.Fprintf(h, "%v\n", p.cfg.Args)
	return &internal.CacheKey{
		Key: fmt.Sprintf("credhelper/%s/%s", strings.ToLower(p.cfg.Protocol.String()), hex.EncodeToString(h.Sum(nil))),
	}, nil
}

// RefreshToken implements internal.TokenProvider.
func (p *credHelperTokenProvider) RefreshToken(ctx context.Context, _, _ *internal.Token) (*internal.Token, error) {
	// Minting and refreshing is the same thing: an invocation of the helper.
	return p.MintToken(ctx, nil)
}

// MintToken implements internal.TokenProvider.
func (p *credHelperTokenProvider) MintToken(ctx context.Context, _ *internal.Token) (*internal.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer

	args := append([]string{p.cfg.Exec}, p.cfg.Args...)
	cmd := exec.CommandContext(ctx, p.cfg.Exec, p.cfg.Args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := clock.Now(ctx)
	logging.Debugf(ctx, "Running the credential helper: %q", args)
	err := cmd.Run()
	logging.Debugf(ctx, "Credential helper call finished in %s", clock.Since(ctx, start))

	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		logCredHelperErr(ctx, "Credential helper call failed", err, &stderr)
		return nil, errors.Fmt("running the credential helper: %w", err)
	}

	tok, err := p.handler(stdout.Bytes())
	if err != nil {
		logCredHelperErr(ctx, "Parsing the credential helper output", err, &stderr)
		return nil, errors.Fmt("parsing the credential helper output: %w", err)
	}

	if dt := tok.Expiry.Sub(clock.Now(ctx)); dt < 0 {
		logCredHelperErr(ctx, "The credential helper is misbehaving", errors.Fmt("the returned token is already expired"), &stderr)
		return nil, errors.Fmt("the credential helper returned expired token (it expired %s ago)", -dt)
	}

	return &internal.Token{
		Token:   *tok,
		IDToken: internal.NoIDToken,
		Email:   internal.UnknownEmail,
	}, nil
}

func logCredHelperErr(ctx context.Context, msg string, err error, stderr *bytes.Buffer) {
	if stderrStr := strings.TrimSpace(stderr.String()); stderrStr != "" {
		logging.Errorf(ctx, "%s: %s:\n%s", msg, err, stderrStr)
	} else {
		logging.Errorf(ctx, "%s: %s", msg, err)
	}
}

////////////////////////////////////////////////////////////////////////////////

// externalHandler picks an output handler based on the protocol.
func externalHandler(protocol credhelperpb.Protocol) (func([]byte) (*oauth2.Token, error), error) {
	switch protocol {
	case credhelperpb.Protocol_RECLIENT:
		return reclientHandler, nil
	default:
		return nil, errors.Fmt("unrecognized credential helper protocol %q", protocol)
	}
}

// reclientHandler parses reclient-compatible credential helper output.
func reclientHandler(stdout []byte) (*oauth2.Token, error) {
	var out struct {
		Token   string            `json:"token"`
		Headers map[string]string `json:"headers"`
		Expiry  string            `json:"expiry"`
	}
	if err := json.Unmarshal(stdout, &out); err != nil {
		return nil, errors.Fmt("not a valid JSON output format: %w", err)
	}
	switch {
	case out.Token == "":
		return nil, errors.Fmt("no \"token\" field in the output")
	case out.Expiry == "":
		return nil, errors.Fmt("no \"expiry\" field in the output")
	case len(out.Headers) != 0:
		switch auth := out.Headers["Authorization"]; {
		case auth == "":
			return nil, errors.Fmt("no \"Authorization\" header in the output")
		case auth != fmt.Sprintf("Bearer %s", out.Token):
			return nil, errors.Fmt("\"Authorization\" header value doesn't match the token")
		case len(out.Headers) > 1:
			return nil, errors.Fmt("the output has headers other than \"Authorization\", this is not supported")
		}
	}
	expiry, err := time.Parse(time.UnixDate, out.Expiry)
	if err != nil {
		return nil, errors.Fmt("bad expiry time format %q, must match %q", out.Expiry, time.UnixDate)
	}
	return &oauth2.Token{
		AccessToken: out.Token,
		TokenType:   "Bearer",
		Expiry:      expiry.UTC(),
	}, nil
}
