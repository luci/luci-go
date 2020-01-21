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

package usercontent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

const isolatePathPattern = "/isolate/:host/:ns/:digest"

// GenerateSignedIsolateURL returns a signed 1h-lived URL at which the
// content of the given isolated file can be fetched via plain HTTP.
func (s *Server) GenerateSignedIsolateURL(ctx context.Context, isolateHost, ns, digest string) (u *url.URL, expiration time.Time, err error) {
	return s.generateSignedURL(ctx, fmt.Sprintf("/isolate/%s/%s/%s", isolateHost, ns, digest))
}

func (s *Server) handleIsolateContent(ctx *router.Context) {
	// the path parameters must be valid because we validated the token that is
	// based on the path. Presumably we never generate invalid paths.

	host := ctx.Params.ByName("host")
	ns := ctx.Params.ByName("ns")
	digest := ctx.Params.ByName("digest")

	fetchIsolate := s.testFetchIsolate
	if fetchIsolate == nil {
		fetchIsolate = s.fetchIsolate
	}
	w := &writerChecker{w: ctx.Writer}
	err := fetchIsolate(ctx.Context, host, ns, digest, w)
	httpStatus, isHTTPErr := lhttp.IsHTTPError(err)
	switch {
	case err == nil:
	// Great.

	case w.Called():
		// Too late to write anything else.
		logging.Errorf(ctx.Context, "failed to write isolate content midflight: %s", err)

	case isHTTPErr && httpStatus == http.StatusNotFound:
		http.Error(ctx.Writer, err.Error(), http.StatusNotFound)

	default:
		logging.Errorf(ctx.Context, "internal error while serving isolate content: %s", err)
		http.Error(ctx.Writer, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) fetchIsolate(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
	client := isolatedclient.New(s.anonClient, s.authClient, "https://"+isolateHost, ns, nil, nil)
	return client.Fetch(ctx, isolated.HexDigest(digest), w)
}

type writerChecker struct {
	w      io.Writer
	called int32
}

func (w *writerChecker) Write(p []byte) (int, error) {
	atomic.StoreInt32(&w.called, 1)
	return w.w.Write(p)
}

func (w *writerChecker) Called() bool {
	return atomic.LoadInt32(&w.called) == 1
}
