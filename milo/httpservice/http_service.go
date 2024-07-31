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

package httpservice

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"

	configpb "go.chromium.org/luci/milo/proto/config"
)

// HTTPService is the Milo frontend service that serves multiple regular HTTP
// endpoints.
// TODO(weiweilin): move other HTTP endpoints to HTTPService.
type HTTPService struct {
	Server *server.Server

	// GetSettings returns the current setting for milo.
	GetSettings func(c context.Context) (*configpb.Settings, error)

	// GetResultDBClient returns a ResultDB client for the given context.
	GetResultDBClient func(c context.Context, host string, as auth.RPCAuthorityKind) (rdbpb.ResultDBClient, error)
}

// RegisterRoutes registers routes explicitly handled by the handler.
func (s *HTTPService) RegisterRoutes() {
	baseMW := router.NewMiddlewareChain()
	baseAuthMW := baseMW.Extend(
		middleware.WithContextTimeout(time.Minute),
		auth.Authenticate(s.Server.CookieAuth),
	)

	s.Server.Routes.GET("/raw-artifact/*artifactName", baseAuthMW, handleError(s.buildRawArtifactHandler("/raw-artifact/")))
	s.Server.Routes.GET("/configs.js", baseMW, handleError(s.configsJSHandler))
}

// handleError is a wrapper for a handler so that the handler can return an error
// rather than call ErrorHandler directly.
// This should be used for handlers that render webpages.
func handleError(handler func(c *router.Context) error) func(c *router.Context) {
	return func(c *router.Context) {
		if err := handler(c); err != nil {
			ErrorHandler(c, err)
		}
	}
}

// ErrorHandler renders an error page for the user.
func ErrorHandler(c *router.Context, err error) {
	code := grpcutil.Code(err)
	switch code {
	case codes.Unauthenticated:
		loginURL, err := auth.LoginURL(c.Request.Context(), c.Request.URL.RequestURI())
		if err == nil {
			http.Redirect(c.Writer, c.Request, loginURL, http.StatusFound)
			return
		}
		errors.Log(
			c.Request.Context(), errors.Annotate(err, "failed to retrieve login URL").Err())
	case codes.OK:
		// All good.
	default:
		errors.Log(c.Request.Context(), err)
	}

	status := grpcutil.CodeStatus(code)
	c.Writer.WriteHeader(status)
	if _, err := c.Writer.Write([]byte(err.Error())); err != nil {
		logging.Warningf(c.Request.Context(), "failed to write response body: %s", err)
		return
	}
}
