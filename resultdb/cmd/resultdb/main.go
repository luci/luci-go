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

package main

import (
	"context"
	"flag"
	"io"
	"net/url"
	"time"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/usercontent"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// IsolateURLGenerator generates a plain HTTP URL for an isolate file.
type IsolateURLGenerator func(ctx context.Context, host, ns, digest string) (u *url.URL, expiration time.Time, err error)

// resultDBServer implements pb.ResultDBServer.
//
// It does not return gRPC-native errors. NewResultDBServer takes care of that.
type resultDBServer struct {
	generateIsolateURL IsolateURLGenerator
}

// NewResultDBServer creates an implementation of resultDBServer.
func NewResultDBServer(generateIsolateURL IsolateURLGenerator) pb.ResultDBServer {
	return &pb.DecoratedResultDB{
		Service:  &resultDBServer{generateIsolateURL: generateIsolateURL},
		Prelude:  internal.CommonPrelude,
		Postlude: internal.CommonPostlude,
	}
}

func main() {
	insecureSelfURLs := flag.Bool(
		"insecure-self-urls",
		false,
		"Use http:// (not https://) for URLs pointing back to ResultDB",
	)
	contentHostname := flag.String(
		"user-content-host",
		// TODO(crbug.com/1042261): remove the default and make it required.
		// Without the default staging will start crashing.
		"results.usercontent.cr.dev",
		"Use this host for all user-content URLs",
	)

	internal.Main(func(srv *server.Server) error {
		srv.Routes.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
			io.WriteString(c.Writer, "OK")
		})

		contentServer, err := usercontent.NewServer(srv.Context, *insecureSelfURLs, *contentHostname)
		if err != nil {
			return err
		}
		contentServer.InstallHandlers(srv.VirtualHost(*contentHostname))

		pb.RegisterResultDBServer(srv.PRPC, NewResultDBServer(contentServer.GenerateSignedIsolateURL))

		// Register an empty Recorder server only to make the discovery service
		// list it.
		// The actual traffic will be directed to another deployment, i.e. this
		// binary will never see Recorder RPCs.
		// TODO(nodir): replace this hack with a separate discovery Deployment that
		// dynamically fetches discovery documents from other deployments and
		// returns their union.
		pb.RegisterRecorderServer(srv.PRPC, nil)

		srv.PRPC.AccessControl = prpc.AllowOriginAll

		return nil
	})
}
