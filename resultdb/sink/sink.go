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
}

// Server contains state relevant to the server itself.
// It should always be created by a call to NewServer.
// After a call to Serve(), Server will accept connections on its Port and
// gather test results to send to its Recorder.
type Server struct {
	cfg     ServerConfig
	errC    chan error
	doneC   chan struct{}
	httpSrv http.Server

	// 1 indicates that the server is starting or has started. 0, otherwise.
	started int32

	// Listener for tests
	testListener net.Listener
}

// NewServer creates a Server value and populates optional values with defaults.
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	// validate the config first
	if cfg.Recorder == nil {
		return nil, errors.Reason("ServerConfig.Recorder: unspecified").Err()
	}
	if err := pbutil.ValidateInvocationName(cfg.Invocation); err != nil {
		return nil, errors.Annotate(err, "ServerConfig.Invocation").Err()
	}
	if cfg.UpdateToken == "" {
		return nil, errors.Reason("ServerConfig.UpdateToken: unspecified").Err()
	}

	// set the default values for the optional config fields missing.
	srvCfg := cfg
	if srvCfg.Address == "" {
		srvCfg.Address = DefaultAddr
	}
	if srvCfg.AuthToken == "" {
		tk, err := genAuthToken(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "ServerConfig.AuthToken").Err()
		}
		srvCfg.AuthToken = tk
	}

	s := &Server{
		cfg:   srvCfg,
		errC:  make(chan error, 1),
		doneC: make(chan struct{}, 1),
	}
	return s, nil
}

// Done returns a channel that is closed when the server terminates.
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

// ErrC returns a channel that transmits a server error.
//
// Receiving an error on this channel implies that the server has either
// stopped running or is in the process of stopping.
func (s *Server) ErrC() <-chan error {
	return s.errC
}

// Run invokes callback in a context where the server is running.
//
// The context passed to callback will be cancelled if the server has stopped due to
// critical errors or Close being invoked. The context also has the server's information
// exported into it. If callback finishes, Run will return the error the callback returned.
func (s *Server) Run(ctx context.Context, callback func(context.Context) error) error {

	// start a sink server
	err := s.Start(ctx)
	if err != nil {
		return err
	}
	// TODO(ddoman): Run and Start can be invoked multiple times in parallel.
	// s.Start
	//   - starts a singleton sinkServer on the first-time invocation
	//   - returns nil immediately after the first-time invocation
	//   - returns an error once Close is invoked
	// s.Close
	//   - stops the HTTP server
	//
	// s.Run
	//   - invokes Start
	//   - returns an error if the server has stopped already
	//   - can be invoked multiple times in parallel, until the server gets closed.
	//
	// The caller of sink.NewServer is responsible to call Close and wait for Done when
	// the server is no longer needed.

	// launch and wait until callback returns.
	cbCtx, cancel := context.WithCancel(s.Export(ctx))
	defer cancel()
	go func() {
		select {
		case <-s.Done():
			// cancel the context if the server terminates before the callback finishes.
			cancel()
		case <-cbCtx.Done():
		}
	}()
	return callback(cbCtx)
}

// Start runs the server.
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
		var err error
		defer func() {
			// close SinkServer to complete all the outgoing requests
			// before sending a signal to s.errC.
			closeSinkServer(ctx, ss)
			s.errC <- err
			close(s.errC)
			close(s.doneC)
		}()
		if s.testListener == nil {
			err = s.httpSrv.ListenAndServe()
		} else {
			// In test mode, the the listener MUST be prepared already.
			err = s.httpSrv.Serve(s.testListener)
		}
	}()
	return nil
}

// Close stops the server.
//
// SinkServer processes sinkRequests asychronously, and Close doesn't guarantee completion
// of all the scheduled requests. Use Done to ensure that all the scheduled requests have
// been processed.
func (s *Server) Close() error {
	if atomic.LoadInt32(&s.started) == 0 {
		// hasn't been started
		return ErrCloseBeforeStart
	}
	if err := s.httpSrv.Close(); err != nil {
		return err
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

// genAuthToken generates and returns a random auth token.
func genAuthToken(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := cryptorand.Read(ctx, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
