// Copyright 2023 The LUCI Authors.
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

package frontend

import (
	"net/http"
	"net/url"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// buildRawArtifactHandler builds a raw artifact handler for the given path
// prefix.
// The handler makes it possible to have stable, shareable URLs for artifacts.
//
// We implement this in Milo because
// 1. ResultDB doesn't support cookie based authentication nor a signin page.
// 2. Most users should have already signed in to Milo.
//
// The route is expected to have the format: `${prefix}*artifactName`
// For example, if the prefix is "/raw-artifact/", the route should be
// "/raw-artifact/*artifactName".
func (s *HTTPService) buildRawArtifactHandler(prefix string) func(ctx *router.Context) error {
	return func(ctx *router.Context) error {
		// Use EscapedPath so we can obtain the undecoded artifact name.
		path := ctx.Request.URL.EscapedPath()
		// Read artifactName by removing the prefix.
		// We cannot read the artifactName from ctx.Params.ByName("ArtifactName")
		// because the value is decoded automatically, making it impossible to
		// differentiate "a%2fb" and "a/b".
		// This is also why the path prefix is required to build the raw artifact
		// handler.
		// Related: https://github.com/julienschmidt/httprouter/issues/284
		artifactName := path[len(prefix):]

		settings, err := s.GetSettings(ctx.Request.Context())
		if err != nil {
			return errors.Annotate(err, "failed to get Milo's service settings").Err()
		}
		rdbClient, err := s.GetResultDBClient(ctx.Request.Context(), settings.Resultdb.Host, auth.AsSessionUser)
		if err != nil {
			return errors.Annotate(err, "failed to get ResultDB client").Err()
		}

		artifact, err := rdbClient.GetArtifact(ctx.Request.Context(), &resultpb.GetArtifactRequest{Name: artifactName})
		if err != nil {
			return appstatus.GRPCifyAndLog(ctx.Request.Context(), err)
		}

		fetchUrl := artifact.FetchUrl
		parsedFetchUrl, err := url.Parse(fetchUrl)
		if err != nil {
			return errors.Annotate(err, "failed to parse artifact.fetchUrl").Err()
		}
		fetchUrlQuery := parsedFetchUrl.Query()
		// Copy query params from request to fetch URL.
		requestQuery := ctx.Request.URL.Query()
		// We use two for loops as a query keycan have multiple values.
		for key, values := range requestQuery {
			for _, value := range values {
				fetchUrlQuery.Add(key, value)
			}
		}
		parsedFetchUrl.RawQuery = fetchUrlQuery.Encode()

		http.Redirect(ctx.Writer, ctx.Request, parsedFetchUrl.String(), http.StatusFound)
		return nil
	}
}
