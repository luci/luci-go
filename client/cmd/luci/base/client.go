// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// NewResultDBClient creates authenticated ResultDB and Schemas pRPC clients.
func (af *AuthFlags) NewResultDBClient(ctx context.Context, host string) (pb.ResultDBClient, pb.SchemasClient, *http.Client, error) {
	httpClient, err := af.NewHTTPClient(ctx)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "failed to create http client")
	}
	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	return pb.NewResultDBClient(prpcClient), pb.NewSchemasClient(prpcClient), httpClient, nil
}
