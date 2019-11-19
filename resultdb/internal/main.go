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

package internal

import (
	"flag"

	"cloud.google.com/go/spanner"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/span"
)

const (
	// accessGroup is a CIA group that can access ResultDB.
	// TODO(crbug.com/1013316): remove in favor of realms.
	accessGroup = "luci-resultdb-access"
)

// Main runs a service.
//
// Registers -spanner-database flag and initializes a Spanner client.
func Main(init func(srv *server.Server) error) {
	spannerDB := flag.String("spanner-database", "", "Name of the spanner database to connect to")

	server.Main(nil, func(srv *server.Server) error {
		var err error
		if srv.Context, err = withProdSpannerClient(srv.Context, *spannerDB); err != nil {
			return err
		}

		return init(srv)
	})
}

func withProdSpannerClient(ctx context.Context, dbFlag string) (context.Context, error) {
	if dbFlag == "" {
		return ctx, errors.Reason("-spanner-database flag is required").Err()
	}

	// Init a Spanner client.
	spannerClient, err := spanner.NewClient(ctx, dbFlag)
	if err != nil {
		return ctx, err
	}
	return span.WithClient(ctx, spannerClient), nil
}
