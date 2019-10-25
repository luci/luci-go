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
	"errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	// TODO(crbug.com/1017288) Uncomment when crrev.com/c/1876313 lands.
	// sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

// ServerConfig defines the parameters of the server.
type ServerConfig struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder *pb.RecorderClient

	// AuthToken is a secret token to expect from clients. Optional.
	AuthToken string
	// Port is the TCP port to listen on. Optional.
	Port int

	// Invocation is the name of the invocation that test results should append
	// to.
	Invocation string
	// UpdateToken is the token that allows writes to Invocation.
	UpdateToken string

	// TestPathPrefix will be prepended to the test_path of each TestResult.
	TestPathPrefix string
}

// Server contains state relevant to the server itself.
// It should always be created by a call to NewServer.
// After a call to Serve(), Server will accept connections on its Port and
// gather test results to send to its Recorder.
type Server struct{}

// NewServer creates a Server value and populates optional values with defaults.
//
// If cfg.AuthToken is "" it will be randomly generated in a secure way.
func NewServer(cfg ServerConfig) (*Server, error) {
	return nil, errors.New("not implemented yet")
}

// Config retrieves the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
//
// If Port was originally 0, the Serve function will choose a port arbitrarily.
// In that case, Config will only return the chosen Port after a call to Serve.
func (s *Server) Config() ServerConfig {
	return ServerConfig{}
}

// Serve runs the Server and blocks until it stops running.
func (s *Server) Serve() error {
	return errors.New("not implemented yet")
}

// Close tells the Server to shutdown and blocks until it stops running.
//
// The Server will attempt to finish handling any messages that have not been
// processed yet. If ctx is canceled it will immediately abort all operations
// and return from Close as soon as possible.
func (s *Server) Close(ctx context.Context) error {
	return errors.New("not implemented yet")
}

// Process handles a message as if it had been sent over the TCP interface.
// TODO(crbug.com/1017288) Uncomment when crrev.com/c/1876313 lands.
func (s *Server) Process( /*msg *sinkpb.SinkMessageContainer*/ ) error {
	return errors.New("not implemented yet")
}

// Export exports lucictx.ResultSink derived from the server configuration into
// the context.
// TODO(crbug.com/1017288) lucictx.ResultSink does not exist yet.
func (s *Server) Export(ctx context.Context) context.Context {
	return nil
}
