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

// Package srvhttp allows LUCI servers to get an HTTP client that has all
// default monitoring instrumentation installed.
package srvhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.chromium.org/luci/server/auth"
)

// DefaultTransport is a http.RoundTripper to use for HTTP requests done by the
// server.
//
// This round tripper makes anonymous requests (i.e. doesn't attach any
// authentication headers) and it is roughly similar to http.DefaultTransport.
// It is long-lived and can be reused independently of `ctx` deadline or
// cancellation state (`ctx` is only used for its values).
//
// If `ctx` is derived from the server's root context, this round tripper will
// be instrumented with metrics and tracing and will be populating "User-Agent"
// with information about the server.
//
// If `ctx` is not derived from the server's root context (can happen in tests,
// for example), this is literally http.DefaultTransport.
func DefaultTransport(ctx context.Context) http.RoundTripper {
	switch rt, err := auth.GetRPCTransport(ctx, auth.NoAuth); {
	case err == nil:
		return rt
	case errors.Is(err, auth.ErrNotConfigured):
		return http.DefaultTransport
	default:
		panic(fmt.Sprintf("unexpected error in GetRPCTransport: %s", err))
	}
}

// DefaultClient is an HTTP client that uses DefaultTransport(ctx).
func DefaultClient(ctx context.Context) *http.Client {
	return &http.Client{Transport: DefaultTransport(ctx)}
}
