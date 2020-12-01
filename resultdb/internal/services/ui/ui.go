// Copyright 2020 The LUCI Authors.
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

package ui

import (
	"context"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	uipb "go.chromium.org/luci/resultdb/internal/proto/ui"
)

// uiServer implements uipb.UIServer.
//
// It does not return gRPC-native errors; use DecoratedUI with
// internal.CommonPostlude.
type uiServer struct {
	generateArtifactURL func(ctx context.Context, requestHost, artifactName string) (url string, expiration time.Time, err error)
}

// Options is ui server configuration.
type Options struct {
	artifactcontent.Options
}

// InitServer initializes a ui server.
func InitServer(srv *server.Server, opts Options) error {
	contentServer, err := artifactcontent.NewArtifactContentServer(srv.Context, opts.Options)
	if err != nil {
		return errors.Annotate(err, "failed to create an artifact content server").Err()
	}

	// Serve all possible content hostnames.
	hosts := stringset.New(len(opts.ContentHostnameMap))
	for _, v := range opts.ContentHostnameMap {
		hosts.Add(v)
	}
	for _, host := range hosts.ToSortedSlice() {
		contentServer.InstallHandlers(srv.VirtualHost(host))
	}

	uipb.RegisterUIServer(srv.PRPC, &uipb.DecoratedUI{
		Service: &uiServer{
			generateArtifactURL: contentServer.GenerateSignedURL,
		},
		Postlude: internal.CommonPostlude,
	})

	srv.PRPC.AccessControl = prpc.AllowOriginAll
	return nil
}
