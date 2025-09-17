// Copyright 2025 The LUCI Authors.
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

package grpcutil

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/data/stringset"
)

// BranchFilterPredicate is called to check if the interceptor applies.
type BranchFilterPredicate func(ctx context.Context, fullMethod string) bool

// UnaryBranch is passed to UnaryBranchingInterceptor, see its doc.
type UnaryBranch struct {
	// Match is called to check if the interceptor applies.
	Match BranchFilterPredicate
	// Interceptor is the interceptor to apply on a match.
	Interceptor grpc.UnaryServerInterceptor
}

// UnaryBranchingInterceptor returns a UnaryServerInterceptor that checks the
// request against the given list of predicates and forwards it to the first
// matching branch.
//
// If no interceptors match, no interceptors will apply (i.e. the call will
// be forwarded to the actual handler). Use a MatchAny match in the last branch
// to apply a default interceptor if necessary.
//
// Branch filters are checked sequentially one after another. For best
// performance, avoid huge number of branches.
func UnaryBranchingInterceptor(branches []UnaryBranch) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		for _, f := range branches {
			if f.Match(ctx, info.FullMethod) {
				return f.Interceptor(ctx, req, info, handler)
			}
		}
		return handler(ctx, req)
	}
}

// MatchServices produces a branch filter predicate that returns true if the
// RPC call targets any of the RPC services (given by their fully qualified
// names e.g. "proto.package.name.Service").
func MatchServices(svc ...string) BranchFilterPredicate {
	asSet := stringset.NewFromSlice(svc...)
	return func(_ context.Context, fullMethod string) bool {
		// fullMethod looks like "/<service>/<method>". Get "<service>".
		if fullMethod == "" || fullMethod[0] != '/' {
			panic(fmt.Sprintf("unexpected fullMethod %q", fullMethod))
		}
		service := fullMethod[1:strings.LastIndexByte(fullMethod, '/')]
		if service == "" {
			panic(fmt.Sprintf("unexpected fullMethod %q", fullMethod))
		}
		return asSet.Has(service)
	}
}

// MatchAny returns a branch filter predicate that matches any RPC call.
func MatchAny() BranchFilterPredicate {
	return func(context.Context, string) bool { return true }
}
