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

// Package sink provides a server for aggregating test results and sending them
// to the ResultDB backend.
package sink

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync"

	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	// DefaultAddr is the TCP address that the Server listens on by default.
	DefaultAddr = "localhost:62115"
	// AuthTokenHeader is the key of the HTTP request header where the auth token should
	// be specified. Note that the an HTTP request must have the auth token in the header
	// with the following format.
	// Authorization: ResultSink <auth_token>
	AuthTokenKey    = "Authorization"
	AuthTokenPrefix = "ResultSink"
)

// ServerConfig defines the parameters of the server.
type ServerConfig struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder *pb.RecorderClient

	// AuthToken is a secret token to expect from clients. If it is "" then it
	// will be randomly generated in a secure way.
	AuthToken string
	// Address is the HTTP address to listen on. If empty, the server will use
	// the default address.
	Address string

	// Invocation is the name of the invocation that test results should append
	// to.
	Invocation string
	// UpdateToken is the token that allows writes to Invocation.
	UpdateToken string

	// TestIDPrefix will be prepended to the test_id of each TestResult.
	TestIDPrefix string
}

// Server contains state relevant to the server itself.
// It should always be created by a call to NewServer.
// After a call to Serve(), Server will accept connections on its Port and
// gather test results to send to its Recorder.
type Server struct {
	// Context is the root context used by all requests and background activities.
	//
	// This is set with the context given in NewServer call, and can be replaced before
	// Start call.
	Context context.Context
	cfg     ServerConfig
	errC    chan error

	// protects field below
	mu sync.Mutex
	// true inside and after ListenAndServe
	started bool
	// true inside and after Close
	stopped bool

	httpSrv *http.Server
	routes  *router.Router
	prpc    *prpc.Server

	// Listener for tests
	testListener net.Listener
}

// NewServer creates a Server value and populates optional values with defaults.
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	if cfg.AuthToken == "" {
		buf := make([]byte, 32)
		if _, err := cryptorand.Read(ctx, buf); err != nil {
			return nil, err
		}
		cfg.AuthToken = hex.EncodeToString(buf)
	}
	if cfg.Address == "" {
		cfg.Address = DefaultAddr
	}

	s := &Server{
		Context: ctx,
		cfg:     cfg,
		errC:    make(chan error, 1),
		prpc:    &prpc.Server{},
		routes:  router.New(),
	}
	s.routes.Use(router.NewMiddlewareChain(
		// transform panics into HTTP 500
		middleware.WithPanicCatcher,
		// checks the auth-token
		authTokenValidator(s.cfg.AuthToken),
	))
	s.prpc = &prpc.Server{}
	s.httpSrv = &http.Server{
		Addr: cfg.Address,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			s.routes.ServeHTTP(rw, r)
		}),
	}
	return s, nil
}

// Config retrieves the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
func (s *Server) Config() ServerConfig {
	return s.cfg
}

// ErrC returns a channel that transmits a server error.
//
// Receiving an error on this channel implies that the server has either
// stopped running or is in the process of stopping.
func (s *Server) ErrC() <-chan error {
	return s.errC
}

// Run launches the serving loop in a separate goroutine and invokes the callback
// in a context derived from the Server root context.
//
// The context passed to callback will be cancelled if the server encounters an error, or
// server.Close is invoked.
func (s *Server) Run(callback func(context.Context) error) error {
	ctx, cancel := context.WithCancel(s.Context)
	defer cancel()
	// TODO(sajjadm): Add Export here when implemented and explain it in the function comment.
	if err := s.Start(); err != nil {
		return err
	}
	defer s.Close()

	done := make(chan error)
	go func() {
		done <- callback(ctx)
	}()

	select {
	case err := <-s.errC:
		cancel()
		<-done
		return err
	case err := <-done:
		return err
	}
}

// Start runs the server.
//
// On success, Start will return nil, and a subsequent error will be sent on
// the server's ErrC channel.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.Reason("cannot call Start twice").Err()
	}
	s.started = true
	s.mu.Unlock()

	go func() {
		var err error
		switch s.testListener {
		case nil:
			err = s.httpSrv.ListenAndServe()
		default:
			// In test mode, the the listener MUST be prepared already.
			err = s.httpSrv.Serve(s.testListener)
		}

		s.mu.Lock()
		s.stopped = true
		s.mu.Unlock()
		s.errC <- err
	}()
	return nil
}

// Close immediately stops the servers and cancels the requests that are being processed.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started && !s.stopped {
		return s.httpSrv.Close()
	}
	return nil
}

// Export exports lucictx.ResultSink derived from the server configuration into
// the context.
func (s *Server) Export(ctx context.Context) context.Context {
	return lucictx.SetResultSink(ctx, &lucictx.ResultSink{
		Address:   s.cfg.Address,
		AuthToken: s.cfg.AuthToken,
	})
}

// AuthTokenValue returns the value of the auth_token HTTP header that all requests must
// have.
func AuthTokenValue(authToken string) string {
	return fmt.Sprintf("%s %s", AuthTokenPrefix, authToken)
}

// send403 logs the error message and sends a 403 response with the message.
func send403(c context.Context, rw http.ResponseWriter, msg string) {
	logging.Errorf(c, msg)
	http.Error(rw, msg, http.StatusForbidden)
}

// authTokenValidator is a factory function returning a middleware that validates
// a given HTTP request with the auth key.
func authTokenValidator(authToken string) func(c *router.Context, next router.Handler) {
	expected := AuthTokenValue(authToken)

	return func(c *router.Context, next router.Handler) {
		tokens, ok := c.Request.Header[AuthTokenKey]
		if !ok {
			send403(c.Context, c.Writer, "auth_token is missing")
			return
		}
		for _, tk := range tokens {
			if tk == expected {
				next(c)
				return
			}
		}
		send403(c.Context, c.Writer, "no valid auth_token found")
	}
}
