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

// Package gcemeta implements a subset of GCE metadata server protocol.
//
// It can be used to "trick" Go and Python libraries that use Application
// Default Credentials into believing they run on GCE so that they request
// OAuth2 tokens via GCE metadata server (which is implemented by us here).
//
// It implements a significant portion of the GCE metadata protocol, but
// populates only a small subset of the metadata values that are commonly
// accessed by tools.
//
// Following features of the protocol are not implemented:
//   - "wait-for-change"
//   - "https://..." endpoints
package gcemeta

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/integration/internal/localsrv"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// TokenGenerator produces access and ID tokens.
//
// The canonical implementation is &auth.TokenGenerator{}.
type TokenGenerator interface {
	// GenerateOAuthToken returns an access token for a combination of scopes.
	GenerateOAuthToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error)
	// GenerateIDToken returns an ID token with the given audience in `aud` claim.
	GenerateIDToken(ctx context.Context, audience string, lifetime time.Duration) (*oauth2.Token, error)
}

// Server runs a local fake GCE metadata server.
type Server struct {
	// Generator is used to obtain OAuth2 and ID tokens.
	Generator TokenGenerator
	// Email is the email associated with generated tokens.
	Email string
	// Scopes is a list of scopes to put into generated OAuth2 tokens.
	Scopes []string
	// MinTokenLifetime is a minimum lifetime left in returned tokens.
	MinTokenLifetime time.Duration
	// Port is a local TCP port to bind to or 0 to allow the OS to pick one.
	Port int

	srv  localsrv.Server
	addr string
}

// Start launches background goroutine with the serving loop.
//
// The provided context is used as base context for request handlers and for
// logging. The server must be eventually stopped with Stop().
//
// Returns "host:port" address of the launched metadata server.
func (s *Server) Start(ctx context.Context) (string, error) {
	root := s.emulatedMetadata()

	addr, err := s.srv.Start(ctx, "gcemeta", s.Port, func(ctx context.Context, l net.Listener, wg *sync.WaitGroup) error {
		s.addr = l.Addr().String()
		srv := http.Server{
			Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
					logging.Fields{
						"panic.error": p.Reason,
					}.Errorf(ctx, "Caught panic during handling of %q: %s\n%s", req.RequestURI, p.Reason, p.Stack)
					http.Error(rw, "Internal Server Error. See logs.", http.StatusInternalServerError)
				})
				wg.Add(1)
				defer wg.Done()
				logging.Debugf(ctx, "Handling %s %s", req.Method, req.RequestURI)
				serveMetadata(root, rw, req.WithContext(ctx))
			}),
		}
		return srv.Serve(l)
	})

	if err != nil {
		return "", errors.Annotate(err, "failed to start the server").Err()
	}
	return addr.String(), nil
}

// Stop closes the listening socket, notifies pending requests to abort and
// stops the internal serving goroutine.
//
// Safe to call multiple times. Once stopped, the server cannot be started again
// (make a new instance of Server instead).
//
// Uses the given context for the deadline when waiting for the serving loop
// to stop.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Stop(ctx)
}

// emulatedMetadata creates emulated metadata tree.
func (s *Server) emulatedMetadata() *node {
	root := &node{kind: kindDict}

	// These are used by gcloud to probe that we are on GCE. Put in some fake
	// values since we don't want real GCE VM's project to be used for anything
	// when running on GCE.
	root.mount("/computeMetadata/v1/project").dict(map[string]generator{
		"numeric-project-id": emit(0),
		"project-id":         emit("none"),
	})

	// These are used by "gcp-metadata" npm package to probe that we are on GCE.
	// Additionally some tools want a real zone when running on GCE, so pick it
	// up if running there.
	root.mount("/computeMetadata/v1/instance").dict(map[string]generator{
		"name": fast(func(ctx context.Context, _ url.Values) (any, error) {
			if s.onGCE() {
				return metadata.InstanceNameWithContext(ctx)
			}
			hostname, err := os.Hostname()
			if err != nil {
				return nil, errors.Annotate(err, "failed to get the hostname").Err()
			}
			hostname, _, _ = strings.Cut(hostname, ".")
			return hostname, nil
		}),
		"zone": fast(func(ctx context.Context, _ url.Values) (any, error) {
			if s.onGCE() {
				zone, err := metadata.ZoneWithContext(ctx)
				if err != nil {
					return nil, err
				}
				return "projects/0/zones/" + zone, nil
			}
			return nil, errors.Reason("Not available when not on GCE").
				Tag(statusTag(http.StatusNotFound)).Err()
		}),
	})

	// Fully emulate service-accounts/... section.
	for _, acc := range []string{s.Email, "default"} {
		root.mount("/computeMetadata/v1/instance/service-accounts/" + acc).dict(map[string]generator{
			"aliases":  emit([]string{"default"}),
			"email":    emit(s.Email),
			"identity": expensive(s.accountIdentity),
			"scopes":   emit(s.Scopes),
			"token":    expensive(s.accountToken),
		})
	}

	return root
}

// onGCE returns true when running in an environment with a metadata server.
//
// Returns false if GCE_METADATA_HOST is set to this server already (to avoid
// infinite recursion).
func (s *Server) onGCE() bool {
	if s.addr != "" && os.Getenv("GCE_METADATA_HOST") == s.addr {
		return false
	}
	return metadata.OnGCE()
}

// accountToken implements "/token" metadata leaf.
func (s *Server) accountToken(ctx context.Context, q url.Values) (any, error) {
	scopesSet := stringset.New(0)
	for _, scope := range strings.Split(q.Get("scopes"), ",") {
		if scope = strings.TrimSpace(scope); scope != "" {
			scopesSet.Add(scope)
		}
	}
	scopes := s.Scopes
	if len(scopesSet) > 0 {
		scopes = scopesSet.ToSortedSlice()
	}
	tok, err := s.Generator.GenerateOAuthToken(ctx, scopes, s.MinTokenLifetime)
	if err != nil {
		return nil, errors.Annotate(err, "failed to mint the token").Err()
	}
	return map[string]any{
		"access_token": tok.AccessToken,
		"expires_in":   time.Until(tok.Expiry) / time.Second,
		"token_type":   "Bearer",
	}, nil
}

// accountToken implements "/identity" metadata leaf.
func (s *Server) accountIdentity(ctx context.Context, q url.Values) (any, error) {
	aud := q.Get("audience")
	if aud == "" {
		return nil, errors.Reason("`audience` is required").
			Tag(statusTag(http.StatusBadRequest)).Err()
	}

	// HACK(crbug.com/1210747): Refuse to serve ID tokens to "gcloud" CLI tool
	// (based on its audience). They are not available everywhere yet, causing
	// tasks that use "gcloud" to fail. Note that "gcloud" handles the HTTP 404
	// just fine by totally ignoring it. It appears "gcloud" requests ID tokens
	// just because it can, not because they are really needed (at least for all
	// current "gcloud" calls from LUCI). This hack can be removed when all tasks
	// that use "gcloud" are in the realms mode.
	if aud == "32555940559.apps.googleusercontent.com" {
		return nil, errors.Reason("Go away: crbug.com/1210747").
			Tag(statusTag(http.StatusNotFound)).Err()
	}

	tok, err := s.Generator.GenerateIDToken(ctx, aud, s.MinTokenLifetime)
	if err != nil {
		return nil, errors.Annotate(err, "failed to mint the token").Err()
	}
	return tok.AccessToken, nil
}
