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

package sink

import (
	"context"
	"errors"

	resultspb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	// TODO(crbug.com/1017288) Uncomment when crrev.com/c/1876313 lands.
	// sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type ServerConfig struct {
	Recorder *resultspb.RecorderClient

	AuthToken string // Secret token to expect from clients. Optional.
	Port      int    // TCP port to listen on. Optional.

	Invocation  string // Name of the invocation.
	UpdateToken string // Token for the Invocation.

	TestPathPrefix string // String to prepend to each TestResult's test_path.
}

type Server struct{}

// Create a Server and populate unset optional values with sane defaults.
//
// If cfg.AuthToken is "" it will be randomly generated in a secure way.
// If cfg.Port is 0 it will be chosen arbitrarily.
func NewServer(cfg ServerConfig) (*Server, error) {
	return nil, errors.New("Not implemented yet.")
}

// Retrieve the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
func (s *Server) Config() ServerConfig {
	return ServerConfig{}
}

// Run the Server and block until it stops running.
func (s *Server) Serve() error {
	return errors.New("Not implemented yet.")
}

// Tell the Server to shutdown and block until it stops running.
//
// The Server will attempt to finish handling any messages that have not been
// processed yet. If ctx is canceled it will immediately abort all operations
// and return from Close as soon as possible.
func (s *Server) Close(ctx context.Context) error {
	return errors.New("Not implemented yet.")
}

// Process a message as if it had been communicated over the TCP interface.
// TODO(crbug.com/1017288) Uncomment when crrev.com/c/1876313 lands.
func (s *Server) Process( /*msg *sinkpb.SinkMessageContainer*/ ) error {
	return nil
}

// Create a copy of ctx that is modified to include this Server's AuthToken and
// Port.
func (s *Server) Export(ctx context.Context) context.Context {
	return nil
}
