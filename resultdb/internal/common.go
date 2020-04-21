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
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
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
func CommonPrelude(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	// Do not GRPCify the returned error, because CommonPostlude does it
	// and it treats generic GRPC errors as internal.

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
//
// Extracts a status using appstatus and returns to the requester.
// If the error is internal or unknown, logs the stack trace.
func CommonPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	return GRPCifyAndLog(ctx, err)
}

// GRPCifyAndLog converts the error to a GRPC error and potentially logs it.
func GRPCifyAndLog(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	s := statusFromError(err)
	if s.Code() == codes.Internal || s.Code() == codes.Unknown {
		errors.Log(ctx, err)
	}
	return s.Err()
}

// statusFromError returns a status to return to the client based on the error.
func statusFromError(err error) *status.Status {
	if s, ok := appstatus.Get(err); ok {
		return s
	}

	if err := errors.Unwrap(err); err == context.DeadlineExceeded || err == context.Canceled {
		return status.FromContextError(err)
	}

	return status.New(codes.Internal, "internal server error")
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
		return appstatus.Errorf(codes.PermissionDenied, "%s is not in %s CIA group", auth.CurrentIdentity(ctx), accessGroup)

	default:
		return nil
	}
}

// AssertUTC panics if t is not UTC.
func AssertUTC(t time.Time) {
	if t.Location() != time.UTC {
		panic("not UTC")
	}
}
