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

// Package usercontent can serve user content via plain HTTP securely.
package usercontent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/common/isolatedclient"

	"go.chromium.org/luci/common/isolated"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tokens"
)

// Server can generate signed content URLs and serve content.
type Server struct {
	// Included in generated signed URLs and required in content requests.
	HostName string

	anonClient *http.Client
	authClient *http.Client

	testFetchIsolate func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error
}

// NewServer creates a Server.
// Parameter hostname is for Server.Hostname.
func NewServer(ctx context.Context, hostname string) (*Server, error) {
	selfTransport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}

	return &Server{
		HostName:   hostname,
		anonClient: http.DefaultClient,
		authClient: &http.Client{Transport: selfTransport},
	}, nil
}

// GenerateSignedIsolateURL returns a signed 1h-lived URL at which the
// content of the given isolated file can be fetched via plain HTTP.
func (s *Server) GenerateSignedIsolateURL(ctx context.Context, isolateHost, ns, digest string) (string, error) {
	return s.generateSignedURL(ctx, fmt.Sprintf("/isolate/%s/%s/%s", isolateHost, ns, digest))
}

var pathTokenKind = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: time.Hour,
	SecretKey:  "user_content",
	Version:    1,
}

func (s *Server) generateSignedURL(ctx context.Context, urlPath string) (string, error) {
	state := []byte(path.Clean(urlPath))
	tok, err := pathTokenKind.Generate(ctx, state, nil, time.Hour)
	if err != nil {
		return "", err
	}

	q := url.Values{}
	q.Set("token", tok)
	u := &url.URL{
		Scheme:   "https",
		Host:     s.HostName,
		Path:     urlPath,
		RawQuery: q.Encode(),
	}
	return u.String(), nil
}

func (s *Server) fetchIsolate(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
	isolateURL := "https://" + isolateHost
	client := isolatedclient.New(s.anonClient, s.authClient, isolateURL, ns, nil, nil)
	return client.Fetch(ctx, isolated.HexDigest(digest), w)
}

func (s *Server) handleIsolateContent(ctx *router.Context) {
	// the path parameters must be valid because we validated the token that is
	// based on the path. Presumably we never generate invalid paths.

	isolateHost := ctx.Params.ByName("host")
	ns := ctx.Params.ByName("ns")
	digest := ctx.Params.ByName("digest")

	fetchIsolate := s.testFetchIsolate
	if fetchIsolate == nil {
		fetchIsolate = s.fetchIsolate
	}
	w := &writerChecker{w: ctx.Writer}
	switch err := fetchIsolate(ctx.Context, isolateHost, ns, digest, w); {
	case err == nil:
	// Great.

	case w.Called():
		// Too late.
		logging.Errorf(ctx.Context, "failed to write isolate content midlight: %s", err)

	case strings.Contains(err.Error(), "HTTP 404"):
		ctx.Writer.WriteHeader(http.StatusNotFound)
		io.WriteString(ctx.Writer, err.Error())

	default:
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
		io.WriteString(ctx.Writer, "Internal server error")
		logging.Errorf(ctx.Context, "internal error while serving isolate content: %s", err)
	}
}

// InstallHandlers installs handlers to serve user content.
func (s *Server) InstallHandlers(ctx context.Context, r *router.Router) error {
	mc := router.NewMiddlewareChain(middleware.RequireHost(s.HostName), validateToken)
	r.GET("/isolate/:host/:ns/:digest", mc, s.handleIsolateContent)
	return nil
}

func validateToken(ctx *router.Context, next router.Handler) {
	token := ctx.Request.URL.Query().Get("token")
	state := []byte(path.Clean(ctx.Request.URL.Path))

	if token == "" {
		ctx.Writer.WriteHeader(http.StatusUnauthorized)
		io.WriteString(ctx.Writer, "missing token query parameters")
		return
	}

	if _, err := pathTokenKind.Validate(ctx.Context, token, state); err != nil {
		logging.Warningf(ctx.Context, "invalid token: %s", err)
		fmt.Println(err, token)
		ctx.Writer.WriteHeader(http.StatusForbidden)
		io.WriteString(ctx.Writer, "invalid token")
		return
	}

	next(ctx)
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
