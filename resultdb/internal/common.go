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
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
)

var httpClientCtxKey = "context key for a *http.Client"

// WithHTTPClient returns a context with the client embedded.
func WithHTTPClient(ctx context.Context, client *http.Client) context.Context {
	return context.WithValue(ctx, &httpClientCtxKey, client)
}

// HTTPClient retrieves the current http.client from the context.
func HTTPClient(ctx context.Context) *http.Client {
	client, ok := ctx.Value(&httpClientCtxKey).(*http.Client)
	if !ok {
		panic("no HTTP client in context")
	}
	return client
}

// CommonPrelude must be used as a prelude in all ResultDB services.
// Verifies access.
func CommonPrelude(ctx context.Context, methodName string, req proto.Message) (retCtx context.Context, err error) {
	defer func() {
		err = grpcutil.GRPCifyAndLogErr(ctx, err)
	}()

	// TODO(crbug.com/1013316): replace this with project-identified transport.
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return ctx, err
	}
	ctx = WithHTTPClient(ctx, &http.Client{Transport: tr})

	if err := verifyAccess(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// CommonPostlude must be used as a postlude in all ResultDB services.
// Extracts an error code from a grpcutil.Tag, logs a
// stack trace and returns a gRPC-native error.
func CommonPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	// Resultdb codebase uses grpcutil tags for error codes.
	// If the error is not explicitly tagged, then it is an internal error.
	// This prevents returning internal gRPC errors to our clients.
	if _, ok := grpcutil.Tag.In(err); !ok {
		err = errors.
			Annotate(err, "Internal server error").
			InternalReason("%s", err).
			Tag(grpcutil.InternalTag).
			Err()
	}

	return grpcutil.GRPCifyAndLogErr(ctx, err)
}

func verifyAccess(ctx context.Context) error {
	// TODO(crbug.com/1013316): use realms.

	// WARNING: removing this restriction requires removing AsSelf HTTP client
	// that is setup in CommonPrelude()
	// DO NOT REMOVE this until that's done.
	switch allowed, err := auth.IsMember(ctx, accessGroup); {
	case err != nil:
		return err

	case !allowed:
		return errors.
			Reason("%s is not in %q CIA group", auth.CurrentIdentity(ctx), accessGroup).
			Tag(grpcutil.PermissionDeniedTag).
			Err()

	default:
		return nil
	}
}
