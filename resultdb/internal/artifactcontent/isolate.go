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

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal"
)

func (r *contentRequest) handleIsolateContent(ctx context.Context, isolateURL string) {
	host, ns, digest, err := internal.ParseIsolateURL(isolateURL)
	if err != nil {
		r.sendError(ctx, err)
		return
	}

	fetchIsolate := r.testFetchIsolate
	if fetchIsolate == nil {
		fetchIsolate = r.fetchIsolate
	}

	wrote := false
	err = fetchIsolate(ctx, host, ns, digest, writer(func(p []byte) (int, error) {
		if !wrote {
			r.WriteHeaders()
		}
		wrote = true
		return r.w.Write(p)
	}))
	httpStatus, isHTTPErr := lhttp.IsHTTPError(err)
	switch {
	case err == nil:
		// Great.

	case wrote:
		// Too late to write anything else.
		logging.Errorf(ctx, "failed to write isolate content midflight: %s", err)

	case isHTTPErr && httpStatus == http.StatusNotFound:
		// The artifact exists, but its content does not and this is a request for
		// content, so it is OK to respond 404 here.
		http.Error(r.w, err.Error(), http.StatusNotFound)

	default:
		r.sendError(ctx, err)
	}
}

func (s *Server) fetchIsolate(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
	client := isolatedclient.NewClient("https://"+isolateHost, isolatedclient.WithAnonymousClient(s.anonClient), isolatedclient.WithAuthClient(s.authClient), isolatedclient.WithNamespace(ns))
	return client.Fetch(ctx, isolated.HexDigest(digest), w)
}

type writer func(p []byte) (int, error)

func (w writer) Write(p []byte) (int, error) {
	return w(p)
}
