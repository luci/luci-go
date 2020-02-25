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

	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/server"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

const (
	// DefaultAddr is the TCP address that the Server listens on by default.
	DefaultAddr = "localhost:62115"
)

// Options are used to specify SinkerServer configuration.
type Options struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder *pb.RecorderClient

	// AuthToken is a secret token to expect from clients. If it is "" then it
	// will be randomly generated in a secure way.
	AuthToken string
	// TCP address where a ResultSink pRPC server is hosted. If empty, it defaults to
	// "localhost:62115".
	Address string
}

// sinkServer implements pb.SinkServer.
type sinkServer struct {
	*Options
}

func newSinkServer(ctx context.Context, opt Options) (*sinkServer, error) {
	if opt.AuthToken == "" {
		buf := make([]byte, 32)
		if _, err := cryptorand.Read(ctx, buf); err != nil {
			return nil, err
		}
		opt.AuthToken = hex.EncodeToString(buf)
	}
	if opt.Address == "" {
		opt.Address = DefaultAddr
	}
	return &sinkServer{Options: &opt}, nil
}

// InitServer initializes a sink server and registers it in the given server.
func InitServer(srv *server.Server, opt Options) error {
	ss, err := newSinkServer(srv.Context, opt)
	if err != nil {
		return err
	}
	srv.AddPort(server.PortOptions{Name: "sink_server", ListenAddr: opt.Address})
	sinkpb.RegisterSinkServer(srv.PRPC, &sinkpb.DecoratedSink{
		Service: ss,
		// TODO(crbug/1017288) - validate requests with auth_token
		Prelude:  internal.CommonPrelude,
		Postlude: internal.CommonPostlude,
	})
	return nil
}

// Export exports lucictx.ResultDB derived from the server configuration into
// the context.
func (s *sinkServer) Export(ctx context.Context) context.Context {
	db := lucictx.ResultDB{
		ResultSink: lucictx.ResultSink{Address: s.Address, AuthToken: s.AuthToken},
	}
	return lucictx.SetResultDB(ctx, &db)
}
