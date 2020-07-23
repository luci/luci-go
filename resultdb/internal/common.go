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

// WithProjectTransport sets an http client in the context using project-based
// auth transport.
func WithProjectTransport(ctx context.Context, project string) (context.Context, error) {
	// If a client is already present in the context, do not replace it, it may be a test.
	if _, ok := ctx.Value(&httpClientCtxKey).(*http.Client); ok == true {
		return ctx, nil
	}

	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}
	return WithHTTPClient(ctx, &http.Client{Transport: tr}), nil
}

// CommonPrelude must be used as a prelude in all ResultDB services.
// It does not verify access, as individual RPCs check permissions via realms,
// or via update token.
func CommonPrelude(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	return ctx, nil
}

// CommonPostlude must be used as a postlude in all ResultDB services.
//
// Extracts a status using appstatus and returns to the requester.
// If the error is internal or unknown, logs the stack trace.
func CommonPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	return appstatus.GRPCifyAndLog(ctx, err)
}

// AssertUTC panics if t is not UTC.
func AssertUTC(t time.Time) {
	if t.Location() != time.UTC {
		panic("not UTC")
	}
}
