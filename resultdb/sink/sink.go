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
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

const (
	// DefaultAddr is the TCP address that the Server listens on by default.
	DefaultAddr = "localhost:62115"
	// AuthTokenKey is the key of the HTTP request header where the auth token should
	// be specified. Note that the an HTTP request must have the auth token in the header
	// with the following format.
	// Authorization: ResultSink <auth_token>
	AuthTokenKey = "Authorization"
	// AuthTokenPrefix is embedded into the value of the Authorization HTTP request header,
	// where the auth_token must be present. For the details about the value format of
	// the Authoization HTTP request header, find the description of `AuthTokenKey`.
	AuthTokenPrefix = "ResultSink"
)

// ErrCloseBeforeStart is returned by Close(), when it was invoked before the server
// started.
var ErrCloseBeforeStart error = errors.Reason("the server is not started yet").Err()

// ServerConfig defines the parameters of the server.
type ServerConfig struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder pb.RecorderClient

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

	// Listener for tests
	testListener net.Listener
}

func (c *ServerConfig) Validate() error {
	if c.Recorder == nil {
		return errors.Reason("Recorder: unspecified").Err()
	}
	if err := pbutil.ValidateInvocationName(c.Invocation); err != nil {
		return errors.Annotate(err, "Invocation").Err()
	}
	if c.UpdateToken == "" {
		return errors.Reason("UpdateToken: unspecified").Err()
	}
	if c.TestIDPrefix != "" {
		if err := pbutil.ValidateTestID(c.TestIDPrefix); err != nil {
			return errors.Annotate(err, "TestIDPrefix").Err()
		}
	}

	return nil
}

// Server contains state relevant to the server itself.
// It should always be created by a call to NewServer.
// After a call to Serve(), Server will accept connections on its Port and
// gather test results to send to its Recorder.
type Server struct {
	cfg     ServerConfig
	doneC   chan struct{}
	httpSrv http.Server

	// 1 indicates that the server is starting or has started. 0, otherwise.
	started int32

	mu  sync.Mutex // protects err
	err error
}

// NewServer creates a Server value and populates optional values with defaults.
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	// validate the config for required fields
	if err := cfg.Validate(); err != nil {
		return nil, errors.Annotate(err, "invalid ServerConfig").Err()
	}

	// set the default values for the optional config fields missing.
	if cfg.Address == "" {
		cfg.Address = DefaultAddr
	}
	if cfg.AuthToken == "" {
		tk, err := genAuthToken(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "failed to generate AuthToken").Err()
		}
		cfg.AuthToken = tk
	}

	s := &Server{
		cfg:   cfg,
		doneC: make(chan struct{}),
	}
	return s, nil
}

// Done returns a channel that is closed when the server terminated and finished processing
// all the ongoing requests.
func (s *Server) Done() <-chan struct{} {
	return s.doneC
}

// Config retrieves the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
func (s *Server) Config() ServerConfig {
	return s.cfg
}

// Err returns a server error, explaining the reason of the sink server closed.
//
// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why.
func (s *Server) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Run starts a server and runs callback in a context where the server is running.
//
// The context passed to callback will be cancelled if the server has stopped due to
// critical errors or Close being invoked. The context also has the server's information
// exported into it. If callback finishes, Run will stop the server and return the error
// callback returned.
func Run(ctx context.Context, cfg ServerConfig, callback func(context.Context, ServerConfig) error) (err error) {
	s, err := NewServer(ctx, cfg)
	if err != nil {
		return err
	}
	if err := s.Start(ctx); err != nil {
		return err
	}
	defer func() {
		// Run returns nil IFF both of the callback and Shutdown returned nil.
		sErr := s.Shutdown(ctx)
		if err == nil {
			err = sErr
		}
	}()

	// It's necessary to create a new context with a new variable. If param `ctx` was
	// re-used, the new context would be passed to Shutdown in the deferred function after
	// being cancelled.
	cbCtx, cancel := context.WithCancel(s.Export(ctx))
	defer cancel()
	go func() {
		select {
		case <-s.Done():
			// cancel the context if the server terminates before callback finishes.
			cancel()
		case <-ctx.Done():
		}
	}()
	return callback(cbCtx, cfg)
}

// Start runs the server.
//
// On success, Start will return nil, and a subsequent error can be obtained from Err.
func (s *Server) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return errors.Reason("cannot call Start twice").Err()
	}

	// launch an HTTP server
	routes := router.NewWithRootContext(ctx)
	routes.Use(router.NewMiddlewareChain(middleware.WithPanicCatcher))
	s.httpSrv.Addr = s.cfg.Address
	s.httpSrv.Handler = routes

	// install a pRPC service into it.
	ss, err := newSinkServer(ctx, s.cfg)
	if err != nil {
		return err
	}
	prpc := &prpc.Server{Authenticator: prpc.NoAuthentication}
	prpc.InstallHandlers(routes, router.MiddlewareChain{})
	sinkpb.RegisterSinkServer(prpc, ss)

	go func() {
		defer func() {
			// close SinkServer to complete all the outgoing requests before closing doneC
			// to send a signal.
			closeSinkServer(ctx, ss)
			close(s.doneC)
		}()

		var err error
		if s.cfg.testListener == nil {
			// err will be written to s.err after the channel fully drained.
			err = s.httpSrv.ListenAndServe()
		} else {
			// In test mode, the the listener MUST be prepared already.
			err = s.httpSrv.Serve(s.cfg.testListener)
		}

		// if the reason of the server stopped was due to s.Close or s.Shutdown invoked,
		// then s.Err should return nil instead.
		if err == http.ErrServerClosed {
			err = nil
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		s.err = err
	}()
	return nil
}

// Shutdown gracefully shuts down the server without interrupting any ongoing requests.
//
// Shutdown works by first closing the listener for incoming requests, and then waiting for
// all the ongoing requests to be processed and pending results to be uploaded. If
// the provided context expires before the shutdown is complete, Shutdown returns
// the context's error.
func (s *Server) Shutdown(ctx context.Context) error {
	if atomic.LoadInt32(&s.started) == 0 {
		// hasn't been started
		return ErrCloseBeforeStart
	}
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		return err
	}

	select {
	case <-s.Done():
		// Shutdown always returns nil as long as the server was closed and drained.
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops the server.
//
// Server processes sinkRequests asynchronously, and Close doesn't guarantee completion
// of all the ongoing requests. After Close returns, wait for Done to be closed to ensure
// that all the pending results have been uploaded.
//
// It's recommended to use Shutdown() instead of Close with Done.
func (s *Server) Close() error {
	if atomic.LoadInt32(&s.started) == 0 {
		// hasn't been started
		return ErrCloseBeforeStart
	}
	return s.httpSrv.Close()
}

// Export exports lucictx.ResultSink derived from the server configuration into
// the context.
func (s *Server) Export(ctx context.Context) context.Context {
	return lucictx.SetResultSink(ctx, &lucictx.ResultSink{
		Address:   s.cfg.Address,
		AuthToken: s.cfg.AuthToken,
	})
}

// genAuthToken generates and returns a random auth token.
func genAuthToken(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := cryptorand.Read(ctx, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
