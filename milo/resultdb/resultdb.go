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

package resultdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/milo/common"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
)

var resultdbContenxtKey = "context key for resultDB"

// ResultDBFactory is a function that returns a buildbucket rpc builds
// client.
type ResultDBFactory func(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (resultpb.ResultDBClient, error)

func ProdResultDBFactory(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (resultpb.ResultDBClient, error) {
	t, err := auth.GetRPCTransport(c, as, opts...)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	return resultpb.NewResultDBPRPCClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    host,
		Options: rpcOpts,
	}), nil
}

// WithResultDBFactory installs a buildbucket rpc builds client in the
// context.
func WithResultDBFactory(c context.Context, factory ResultDBFactory) context.Context {
	return context.WithValue(c, &resultdbContenxtKey, factory)
}

func ResultdbClient(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (resultpb.ResultDBClient, error) {
	factory, ok := c.Value(&resultdbContenxtKey).(ResultDBFactory)
	if !ok {
		return nil, fmt.Errorf("no buildbucket builds client factory found in context")
	}
	return factory(c, host, as, opts...)
}

func GetHost(c context.Context) (string, error) {
	settings := common.GetSettings(c)
	host := settings.GetResultdb().GetHost()
	if host == "" {
		return "", errors.New("missing buildbucket host in settings")
	}
	return settings.Resultdb.Host, nil
}
