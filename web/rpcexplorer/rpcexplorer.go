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

// Package rpcexplorer contains complied RPC Explorer web app.
//
// Linking to this package will add ~1MB to your binary.
package rpcexplorer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/web/rpcexplorer/internal"
)

// AuthMethod implements an authentication method that RPC Explorer will use.
//
// RPC Explorer needs login/logout URLs and a state endpoint that serves tokens.
type AuthMethod interface {
	auth.UsersAPI
	auth.HasStateEndpoint
}

// Install adds routes to serve RPC Explorer web app from "/rpcexplorer".
//
// If auth is nil, a default config will be used which attempts to use the
// server's frontend OAuth client ID in a Javascript-based login flow.
func Install(r *router.Router, auth AuthMethod) {
	r.GET("/rpcexplorer", nil, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusMovedPermanently)
	})

	// Everything under "services/" should load the main web app (it then itself
	// routes the request based on its URL). Everything else is assumed to be
	// a static resource loaded from the assets bundle.
	r.GET("/rpcexplorer/*path", nil, func(c *router.Context) {
		path := strings.TrimPrefix(c.Params.ByName("path"), "/")

		// Forbid loading RPC explorer in an iframe to avoid clickjacking.
		c.Writer.Header().Set("X-Frame-Options", "deny")
		c.Writer.Header().Set("Content-Security-Policy", "frame-ancestors 'none';")

		// The config endpoint tells the RPC Explorer how to do authentication.
		if path == "config" {
			resp, err := getConfigResponse(c.Request.Context(), auth)
			if err != nil {
				http.Error(c.Writer, fmt.Sprintf("Configuration error: %s", err), http.StatusInternalServerError)
			} else {
				blob, err := json.Marshal(resp)
				if err != nil {
					panic(err)
				}
				c.Writer.Header().Set("Content-Type", "application/json")
				c.Writer.Write(blob)
			}
			return
		}

		// Routes are interpreted by Javascript router, all routed paths should
		// return the same index.html that will figure out what to do with them.
		if path == "" || path == "services" || strings.HasPrefix(path, "services/") {
			path = "index.html"
		}

		hash := assetSHA256(path)
		if hash == nil {
			http.Error(c.Writer, "404 page not found", http.StatusNotFound)
			return
		}

		c.Writer.Header().Set("ETag", fmt.Sprintf("%q", hex.EncodeToString(hash)))
		http.ServeContent(
			c.Writer, c.Request, path, time.Time{},
			strings.NewReader(assetString(path)))
	})
}

type configResponse struct {
	LoginURL     string   `json:"loginUrl,omitempty"`
	LogoutURL    string   `json:"logoutUrl,omitempty"`
	AuthStateURL string   `json:"authStateUrl,omitempty"`
	ClientID     string   `json:"clientId,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

func getConfigResponse(ctx context.Context, m AuthMethod) (*configResponse, error) {
	var out configResponse
	var err error

	if m != nil {
		out.AuthStateURL, err = m.StateEndpointURL(ctx)
		switch err {
		case nil:
			out.LoginURL, err = m.LoginURL(ctx, "/rpcexplorer/")
			if err != nil {
				return nil, err
			}
			out.LogoutURL, err = m.LogoutURL(ctx, "/rpcexplorer/")
			if err != nil {
				return nil, err
			}
			// Note: we don't populate ClientID, since it is not needed when using
			// an existing auth method.
			return &out, nil
		case auth.ErrNoStateEndpoint:
			// This is fine, just ignore AuthMethod.
		default:
			return nil, err
		}
	}

	// Use the server's client ID, but fallback to the default one.
	out.ClientID, err = auth.GetFrontendClientID(ctx)
	if err != nil {
		return nil, err
	}
	if out.ClientID == "" {
		out.ClientID = chromeinfra.RPCExplorerClientID
	}

	out.Scopes, err = auth.GetFrontendOAuthScope(ctx)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

// If set, use this local file system path to find the RPC Explorer build.
//
// This is **only** for local development of RPC Explorer.
var devBuildPath = os.Getenv("LUCI_RPCEXPLORER_LOCAL_BUILD_DIST")

func assetSHA256(path string) []byte {
	if devBuildPath == "" {
		return internal.GetAssetSHA256(path)
	}
	blob := assetString(path)
	if blob == "" {
		return nil // there should be no empty files
	}
	digest := sha256.Sum256([]byte(blob))
	return digest[:]
}

func assetString(path string) string {
	if devBuildPath == "" {
		return internal.GetAssetString(path)
	}
	blob, err := os.ReadFile(filepath.Join(devBuildPath, path))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		panic(err)
	}
	return string(blob)
}
