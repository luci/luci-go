// Copyright 2023 The LUCI Authors.
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

// Package rpcs implements public API RPC handlers.
package rpcs

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"
)

var requestStateCtxKey = "swarming.rpcs.RequestState"

// RequestState carries stated scoped to a single RPC handler.
//
// In production produced by ServerInterceptor. In tests can be injected into
// the context via MockRequestState(...).
//
// Use State(ctx) to get the current value.
type RequestState struct {
	// Config is a snapshot of the server configuration when request started.
	Config *cfg.Config
	// ACL can be used to check ACLs.
	ACL *acls.Checker
}

// ServerInterceptor returns an interceptor that initializes per-RPC context.
//
// The interceptor is active only for selected gRPC services. All other RPCs
// are passed through unaffected.
//
// The initialized context will have RequestState populated, use State(ctx) to
// get it.
func ServerInterceptor(cfg *cfg.Provider, services []*grpc.ServiceDesc) grpcutil.UnifiedServerInterceptor {
	serviceSet := stringset.New(len(services))
	for _, svc := range services {
		serviceSet.Add(svc.ServiceName)
	}

	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
		// fullMethod looks like "/<service>/<method>". Get "<service>".
		if fullMethod == "" || fullMethod[0] != '/' {
			panic(fmt.Sprintf("unexpected fullMethod %q", fullMethod))
		}
		service := fullMethod[1:strings.LastIndex(fullMethod, "/")]
		if service == "" {
			panic(fmt.Sprintf("unexpected fullMethod %q", fullMethod))
		}

		if !serviceSet.Has(service) {
			return handler(ctx)
		}

		cfg := cfg.Config(ctx)
		return handler(context.WithValue(ctx, &requestStateCtxKey, &RequestState{
			Config: cfg,
			ACL:    acls.NewChecker(ctx, cfg),
		}))
	}
}

// State accesses the per-request state in the context or panics if it is
// not there.
func State(ctx context.Context) *RequestState {
	state, _ := ctx.Value(&requestStateCtxKey).(*RequestState)
	if state == nil {
		panic("no RequestState in the context")
	}
	return state
}
