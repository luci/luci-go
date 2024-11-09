// Copyright 2022 The LUCI Authors.
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

// Package rpcquota is a wrapper around LUCI quota for ResultDB.
//
// It provides a LUCI module that configures and initializes the LUCI quota
// library with a quotaconfig configservice.  That module also installs a
// UnaryServerInterceptor to automatically check quota for incoming ResultDB
// and Recorder RPCs.
//
// It also automatically falls back to deducting quota from a wildcard policy
// if there's no policy specific to that user.  See UpdateUserQuota for
// details.
package rpcquota

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	quota "go.chromium.org/luci/server/quotabeta"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"

	"go.chromium.org/luci/resultdb/internal/tracing"
)

var quotaResultCounter = metric.NewCounter(
	"resultdb/rpc/quota",
	"RPC request quota check results",
	nil,
	field.String("service"),     // RPC service
	field.String("method"),      // RPC method
	field.Bool("wildcard_user"), // Did this use the ${user} policy?
	field.Bool("sufficient"),    // Did this request have enough quota?
)

// quotaCheckInterceptor returns a gPRC interceptor to check LUCI quota for
// ResultDB and Recorder RPCs.
func quotaCheckInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		parts := strings.Split(info.FullMethod, "/")
		if len(parts) != 3 || parts[0] != "" {
			panic(fmt.Sprintf("unexpected format of info.FullMethod: %q", info.FullMethod))
		}
		service, method := parts[1], parts[2]
		if service == "luci.resultdb.v1.ResultDB" || service == "luci.resultdb.v1.Recorder" {
			cost := int64(1)
			if method == "BatchGetTestVariants" {
				cost = 10
			}
			if err := UpdateUserQuota(ctx, "rpc_10min/", cost, service, method); err != nil {
				return nil, err
			}
		}
		return handler(ctx, req)
	}
}

var disallowedPolicyNameChars = regexp.MustCompile(`[^A-Za-z0-9-_/]`)

// Make a user identity string suitable for a LUCI quota policy.
func mangleUser(user string) string {
	return disallowedPolicyNameChars.ReplaceAllLiteralString(user, "/")
}

// UpdateUserQuota is a ResultDB-specific wrapper around quota.UpdateQuota.
//
// Its main purpose is to add a 2-stage policy lookup: first look for a
// user-specific policy, and iff none found fall back to a default user policy.
// This is used to control quota granted to different service accounts.
//
// For example, given a config with 2 policies like:
//
// - name: RPC/hourly/${user}, resources: 100, replenishment: 10
// - name: RPC/hourly/user/alice/example.com, resources: 200, replenishment: 10
//
// The outcome of UpdateUserQuota(ctx, "RPC/hourly/", 1) will depend on the
// currently authenticated identity.  If it is 'user:alice@example.com', the
// quota will be debited from a quota bucket called
// "RPC/hourly/user/alice/example.com" (initialized with 200 resources).
// However for 'user:bob@example.com' it would be debited from a bucket called
// "RPC/hourly/user/bob/example.com" (initialized with 100).
//
// In this way, we can set a default policy to apply to any user, and configure
// overrides for specific users that need more (or less).  This includes
// allowing overrides for "anonymous:anonymous".  Note that policy names must
// only use characters in the set [A-Za-z0-9-_/] (aside from the special
// substring ${user}), so user strings in policies should have other characters
// replaced with /.
//
// Note that unlike quota.UpdateQuota this only supports updating a single
// resource at a time; this is sufficent for ResultDB's needs, and keeps the
// fallback logic simple.
//
// This also handles transforming ErrInsufficientQuota into a ResourceExhausted
// appstatus.
//
// - TODO: automatic opts.RequestID..?
func UpdateUserQuota(ctx context.Context, resourcePrefix string, cost int64, service string, method string) (err error) {
	ctx, ts := tracing.Start(ctx, "rpcquota.UpdateUserQuota")
	defer func() { tracing.End(ts, err) }()
	who := mangleUser(string(auth.CurrentIdentity(ctx))) // TODO: should this be PeerIdentity?
	wildcard := false
	// Try deduct from current user first.  If no policy exists for this
	// user this fails quickly, because policies are in in-process memory.
	err = quota.UpdateQuota(ctx, map[string]int64{resourcePrefix + who: -cost}, nil)
	if errors.Unwrap(err) == quotaconfig.ErrNotFound {
		// Fallback to generic ${user} policy.
		wildcard = true
		err = quota.UpdateQuota(ctx, map[string]int64{resourcePrefix + "${user}": -cost}, &quota.Options{User: who})
		if errors.Unwrap(err) == quotaconfig.ErrNotFound {
			// If there's no wildcard config either then deny this.
			err = quota.ErrInsufficientQuota
		}
	}
	quotaResultCounter.Add(ctx, 1, service, method, wildcard, err != quota.ErrInsufficientQuota)
	ts.SetAttributes(attribute.Bool("wildcard", wildcard))
	if err == quota.ErrInsufficientQuota {
		if ctx.Value(&quotaTrackOnlyKey) != nil {
			// Suppress the quota error.
			err = nil
		} else {
			return appstatus.Errorf(codes.ResourceExhausted, "not enough RPC quota")
		}
	}
	return err
}
