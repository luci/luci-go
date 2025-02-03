// Copyright 2018 The LUCI Authors.
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

// Package firebase implements an auth server that allows firebase-tools
// to use an exposed OAuth2 TokenSource for auth.
// See firebase auth documentation at https://github.com/firebase/firebase-tools
// and auth implementation https://github.com/firebase/firebase-tools/blob/9422490bd87e934a097a110f77eddac799d965a4/lib/auth.js
package firebase

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/integration/internal/localsrv"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// Server runs a local server that handles requests to token_uri.
type Server struct {
	// Source is used to obtain OAuth2 tokens.
	Source oauth2.TokenSource
	// Port is a local TCP port to bind to or 0 to allow the OS to pick one.
	Port int

	srv localsrv.Server
}

// Start launches background goroutine with the serving loop.
//
// The provided context is used as base context for request handlers and for
// logging. The server must be eventually stopped with Stop().
//
// Returns the root URL of a local server to use as FIREBASE_TOKEN_URL.
func (s *Server) Start(ctx context.Context) (string, error) {
	// Launch the server to get the port number.
	addr, err := s.srv.Start(ctx, "firebase-auth", s.Port, func(c context.Context, l net.Listener, wg *sync.WaitGroup) error {
		return s.serve(c, l, wg)
	})
	if err != nil {
		return "", errors.Annotate(err, "failed to start the server").Err()
	}
	return fmt.Sprintf("http://%s", addr), nil
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

// serve runs the serving loop.
func (s *Server) serve(ctx context.Context, l net.Listener, wg *sync.WaitGroup) error {
	mux := http.NewServeMux()

	mux.Handle("/oauth2/v3/token", &handler{ctx, wg, func(rw http.ResponseWriter, r *http.Request) {
		err := s.handleTokenRequest(rw, r)

		code := 0
		msg := ""
		if transient.Tag.In(err) {
			code = http.StatusInternalServerError
			msg = fmt.Sprintf("Transient error - %s", err)
		} else if err != nil {
			code = http.StatusBadRequest
			msg = fmt.Sprintf("Bad request - %s", err)
		}

		if code != 0 {
			logging.Errorf(ctx, "%s", msg)
			http.Error(rw, msg, code)
		}
	}})

	srv := http.Server{Handler: mux}
	return srv.Serve(l)
}

// handleTokenRequest handles the OAuth2 flow.
//
// The body of the request is documented here (among many other places):
//
//	https://developers.google.com/identity/protocols/OAuth2InstalledApp#offline
//
// We ignore client_id and client_secret, since we aren't really running OAuth2.
func (s *Server) handleTokenRequest(rw http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	// We support only refreshing access token via 'refresh_token' grant.
	if r.PostFormValue("grant_type") != "refresh_token" {
		return fmt.Errorf("expecting 'refresh_token' grant type")
	}

	// Grab an access token through the source and return it.
	tok, err := s.Source.Token()
	if err != nil {
		return err
	}
	rw.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(rw).Encode(map[string]any{
		"access_token": tok.AccessToken,
		"expires_in":   clock.Until(ctx, tok.Expiry) / time.Second,
		"token_type":   "Bearer",
	})
}

// handler implements http.Handler by wrapping the given handler with some
// housekeeping stuff.
type handler struct {
	ctx     context.Context
	wg      *sync.WaitGroup
	handler http.HandlerFunc
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.wg.Add(1)
	defer h.wg.Done()

	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		p.Log(h.ctx, "Caught panic during handling of %q: %s", r.RequestURI, p.Reason)
		http.Error(rw, "Internal Server Error. See logs.", http.StatusInternalServerError)
	})

	logging.Debugf(h.ctx, "Handling %s %s", r.Method, r.RequestURI)
	h.handler(rw, r.WithContext(h.ctx))
}
