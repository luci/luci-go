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
// The implemented subset of the protocol is very limited. Only a few endpoints
// commonly used to bootstrap GCE auth are supported, and their response format
// is not tweakable (i.e. alt=json or alt=text have no effect).
package gcemeta

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"

	"go.chromium.org/luci/auth/integration/internal/localsrv"
)

// Server runs a local fake GCE metadata server.
type Server struct {
	// Source is used to obtain OAuth2 tokens.
	Source oauth2.TokenSource
	// Email is the email associated with the token.
	Email string
	// Scopes is a list of scopes associated with the token.
	Scopes []string
	// Port is a local TCP port to bind to or 0 to allow the OS to pick one.
	Port int

	srv localsrv.Server
}

// Start launches background goroutine with the serving loop.
//
// The provided context is used as base context for request handlers and for
// logging. The server must be eventually stopped with Stop().
//
// Returns "host:port" address of the launched metadata server.
func (s *Server) Start(ctx context.Context) (string, error) {
	mux := http.NewServeMux()
	s.installRoutes(mux)
	addr, err := s.srv.Start(ctx, "gcemeta", s.Port, func(c context.Context, l net.Listener, wg *sync.WaitGroup) error {
		srv := http.Server{Handler: &handler{c, wg, mux}}
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

// installRoutes populates the muxer.
func (s *Server) installRoutes(mux *http.ServeMux) {
	// This is used by oauth2client to probe that we are on GCE.
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		if subtreeRootOnly(rw, r) {
			replyList(rw, []string{"computeMetadata/"})
		}
	})

	// These are used by gcloud to probe that we are on GCE.
	mux.HandleFunc("/computeMetadata/v1/project/numeric-project-id", func(rw http.ResponseWriter, r *http.Request) {
		replyText(rw, "0")
	})
	mux.HandleFunc("/computeMetadata/v1/project/project-id", func(rw http.ResponseWriter, r *http.Request) {
		replyText(rw, "none")
	})
	mux.HandleFunc("/computeMetadata/v1/instance/service-accounts/", func(rw http.ResponseWriter, r *http.Request) {
		if subtreeRootOnly(rw, r) {
			replyList(rw, []string{s.Email + "/", "default/"})
		}
	})

	for _, acc := range []string{s.Email, "default"} {
		// Used by oauth2client to fetch the list of scopes.
		mux.HandleFunc("/computeMetadata/v1/instance/service-accounts/"+acc+"/", s.accountInfoHandler)
		// Used by gcloud when listing accounts.
		mux.HandleFunc("/computeMetadata/v1/instance/service-accounts/"+acc+"/email", s.accountEmailHandler)
		// Used (at least) by gsutil instead of '/?recursive=True'.
		mux.HandleFunc("/computeMetadata/v1/instance/service-accounts/"+acc+"/scopes", s.accountScopesHandler)
		// Used to actually mint tokens.
		mux.HandleFunc("/computeMetadata/v1/instance/service-accounts/"+acc+"/token", s.accountTokenHandler)
	}
}

func (s *Server) accountInfoHandler(rw http.ResponseWriter, r *http.Request) {
	if !subtreeRootOnly(rw, r) {
		return
	}
	// No one should be calling this handler without /?recursive=True, since it is
	// pretty useless in the non-recursive mode. Add a check just in case.
	if rec := strings.ToLower(r.FormValue("recursive")); rec != "true" && rec != "1" {
		http.Error(rw, "Expected /?recursive=true call", http.StatusBadRequest)
		return
	}
	replyJSON(rw, map[string]interface{}{
		"aliases": []string{"default"},
		"email":   s.Email,
		"scopes":  s.Scopes,
	})
}

func (s *Server) accountEmailHandler(rw http.ResponseWriter, r *http.Request) {
	replyText(rw, s.Email)
}

func (s *Server) accountScopesHandler(rw http.ResponseWriter, r *http.Request) {
	replyList(rw, s.Scopes)
}

func (s *Server) accountTokenHandler(rw http.ResponseWriter, r *http.Request) {
	tok, err := s.Source.Token()
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to mint the token - %s", err), http.StatusInternalServerError)
		return
	}
	replyJSON(rw, map[string]interface{}{
		"access_token": tok.AccessToken,
		"expires_in":   time.Until(tok.Expiry) / time.Second,
		"token_type":   "Bearer",
	})
}

////////////////////////////////////////////////////////////////////////////////

// subtreeRootOnly fails with HTTP 404 if request URI doesn't end with '/'.
//
// This is workaround for stupid http.ServeMux behavior that routes "<stuff>/*"
// to "<stuff>/" handler.
func subtreeRootOnly(rw http.ResponseWriter, r *http.Request) bool {
	if strings.HasSuffix(r.URL.Path, "/") {
		return true
	}
	http.Error(rw, "Unsupported metadata call", http.StatusNotFound)
	return false
}

func replyText(rw http.ResponseWriter, text string) {
	rw.Header().Set("Content-Type", "application/text")
	rw.Write([]byte(text))
}

func replyList(rw http.ResponseWriter, list []string) {
	replyText(rw, strings.Join(list, "\n")+"\n")
}

func replyJSON(rw http.ResponseWriter, obj interface{}) {
	rw.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(rw).Encode(obj)
	if err != nil {
		panic(err)
	}
}

////////////////////////////////////////////////////////////////////////////////

// handler implements http.Handler by wrapping the given handler and adding some
// common logic.
type handler struct {
	ctx context.Context
	wg  *sync.WaitGroup
	h   http.Handler
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.wg.Add(1)
	defer h.wg.Done()

	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		logging.Fields{
			"panic.error": p.Reason,
		}.Errorf(h.ctx, "Caught panic during handling of %q: %s\n%s", r.RequestURI, p.Reason, p.Stack)
		http.Error(rw, "Internal Server Error. See logs.", http.StatusInternalServerError)
	})

	logging.Debugf(h.ctx, "Handling %s %s", r.Method, r.RequestURI)

	// See https://cloud.google.com/compute/docs/storing-retrieving-metadata#querying
	if fl := r.Header.Get("Metadata-Flavor"); fl != "Google" {
		http.Error(rw, fmt.Sprintf("Bad Metadata-Flavor: got %q, want %q", fl, "Google"), http.StatusBadRequest)
		return
	}
	if ff := r.Header.Get("X-Forwarded-For"); ff != "" {
		http.Error(rw, fmt.Sprintf("Forbidden X-Forwarded-For header %q", ff), http.StatusBadRequest)
		return
	}

	rw.Header().Set("Metadata-Flavor", "Google")
	h.h.ServeHTTP(rw, r.WithContext(h.ctx))
}
