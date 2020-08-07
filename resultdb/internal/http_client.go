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

package internal

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

var httpClientCtxKey = "context key for a *http.Client"
var AcknowledgeRisks = "context key for a positive acknowledgement of the risks of using auth.AsSelf"

// MustGetContextHTTPClient retrieves the current http.client from the context.
func MustGetContextHTTPClient(ctx context.Context) *http.Client {
	client, ok := ctx.Value(&httpClientCtxKey).(*http.Client)
	if !ok {
		panic("no HTTP client in context")
	}
	return client
}

// WithSelfTransport sets an http client in the context using the service's own account.
// Only use in deriver module.
func WithSelfTransport(ctx context.Context) (context.Context, error) {
	// If a client is already present in the context, do not replace it, it may be a test.
	if _, ok := ctx.Value(&httpClientCtxKey).(*http.Client); ok {
		return ctx, nil
	}

	// Avoid accidental use of this function by requiring a special value in the context.
	if ack := ctx.Value(&AcknowledgeRisks); ack == nil {
		return nil, errors.Reason("Do not use WithSelfTransport").Err()
	}

	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, &httpClientCtxKey, &http.Client{Transport: tr}), nil
}

// WithTestHTTPClient sets the supplied http client in the context for testing.
func WithTestHTTPClient(ctx context.Context, client *http.Client) context.Context {
	return context.WithValue(ctx, &httpClientCtxKey, client)
}
