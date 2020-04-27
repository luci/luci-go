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

package artifactcontent

import (
	"context"
	"io"
	"net/http"

	"go.chromium.org/luci/grpc/appstatus"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal"
)

// handleIsolateContent serves isolated file content.
func (r *contentRequest) handleIsolateContent(ctx context.Context, isolateURL string) {
	fetchIsolate := r.testFetchIsolate
	if fetchIsolate == nil {
		fetchIsolate = r.fetchIsolate
	}

	// Do not write content headers until we are sure that the isolated file still
	// exists.
	wrote := false
	err := fetchIsolate(ctx, isolateURL, writer(func(p []byte) (int, error) {
		if !wrote {
			r.writeContentHeaders()
		}
		wrote = true
		return r.w.Write(p)
	}))
	httpStatus, isHTTPErr := lhttp.IsHTTPError(err)
	switch {
	case err == nil:
		// Perfect.

	case wrote:
		// The response status/header is already written, and a part of the response
		// body is likely to be written too, so it is too late to write a different
		// status/header/body. Just log the error.
		logging.Errorf(ctx, "failed to write isolate content midflight: %s", err)

	case isHTTPErr && httpStatus == http.StatusNotFound:
		// The artifact exists, but its content does not and this is a request for
		// content, so it is OK to respond 404 here.
		r.sendError(ctx, appstatus.Attachf(err, codes.NotFound, "not found"))

	default:
		r.sendError(ctx, err)
	}
}

func (s *Server) fetchIsolate(ctx context.Context, isolateURL string, w io.Writer) error {
	host, ns, digest, err := internal.ParseIsolateURL(isolateURL)
	if err != nil {
		return err
	}

	client := isolatedclient.NewClient(
		"https://"+host,
		isolatedclient.WithAnonymousClient(s.anonClient),
		isolatedclient.WithAuthClient(s.authClient),
		isolatedclient.WithNamespace(ns),
	)
	return client.Fetch(ctx, isolated.HexDigest(digest), w)
}

type writer func(p []byte) (int, error)

func (w writer) Write(p []byte) (int, error) {
	return w(p)
}
