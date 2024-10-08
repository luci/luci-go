// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Executable gaeshim implements very minimal LUCI Config v1 API by proxying it
// to the v2 server.
//
// It is deployed to GAE where LUCI Config v1 used to run. It is hit by old
// clients that could not migrate to LUCI Config v2 before the v1 was turned
// off.
//
// It also serves redirects to the v2 UI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"
)

func main() {
	var destHost string
	flag.StringVar(&destHost, "config-v2-host", "", "LUCI Config v2 hostname")

	server.Main(nil, nil, func(srv *server.Server) error {
		if destHost == "" {
			return errors.Reason("-config-v2-host is required").Err()
		}

		client, err := remote.New(srv.Context, remote.Options{
			Host:      destHost,
			Creds:     forwardingCreds{},
			UserAgent: srv.UserAgent(),
		})
		if err != nil {
			return err
		}

		// To handle URLs like "/#/projects/something" we'll need to do the redirect
		// from Javascript (since only the frontend can see URL fragments).
		escaped, _ := json.Marshal(destHost)
		frontendRedirectPage := fmt.Sprintf(frontendRedirectHTML, escaped)
		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			ctx.Writer.Header().Set("Content-Type", "text/html; charset=utf8")
			_, _ = ctx.Writer.Write([]byte(frontendRedirectPage))
		})

		// E.g. ".../schemas/projects:luci-logdog.cfg"
		srv.Routes.GET("/schemas/*SchemaName", nil, func(ctx *router.Context) {
			http.Redirect(
				ctx.Writer,
				ctx.Request,
				fmt.Sprintf("https://%s/schemas/%s", destHost, ctx.Params.ByName("SchemaName")),
				http.StatusMovedPermanently,
			)
		})

		// This serves a very truncated Cloud Endpoints discovery doc for v1 API.
		// Only the method we proxy is exposed.
		srv.Routes.GET("/_ah/api/discovery/v1/apis/config/v1/rest", nil, func(ctx *router.Context) {
			ctx.Writer.Header().Set("Content-Type", "application/json")
			rootURL := fmt.Sprintf("https://%s", ctx.Request.Host)
			_, _ = ctx.Writer.Write([]byte(fmt.Sprintf(discoveryDoc, rootURL, rootURL)))
		})

		// An emulation of get_project_configs v1 API method.
		srv.Routes.GET("/_ah/api/config/v1/configs/projects/*Path", nil, func(ctx *router.Context) {
			cfgs, err := getProjectConfigs(
				ctx.Request.Context(),
				client,
				strings.TrimPrefix(ctx.Params.ByName("Path"), "/"),
				ctx.Request.Header,
			)
			if err != nil {
				http.Error(ctx.Writer, err.Error(), grpcutil.CodeStatus(status.Code(err)))
			} else {
				blob, err := json.Marshal(struct {
					Configs []legacyConfig `json:"configs"`
				}{cfgs})
				if err != nil {
					http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
				} else {
					ctx.Writer.Header().Set("Content-Type", "application/json")
					_, _ = ctx.Writer.Write(blob)
				}
			}
		})

		// Everything else is not available.
		srv.Routes.NotFound(nil, func(ctx *router.Context) {
			http.Error(ctx.Writer, fmt.Sprintf("LUCI Config v1 API is no longer supported, use v2 API at https://%s", destHost), http.StatusNotImplemented)
		})

		return nil
	})
}

var forwardedCredsCtxKey = "used by forwardingCreds"

// forwardingCreds implements PerRPCCredentials by forwarding credentials stored
// in the context (if any).
type forwardingCreds struct {
}

// GetRequestMetadata is part of PerRPCCredentials interface.
func (forwardingCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	creds := ctx.Value(&forwardedCredsCtxKey)
	if creds == nil {
		panic("not a request context")
	}
	return creds.(map[string]string), nil
}

// RequireTransportSecurity is part of PerRPCCredentials interface.
func (forwardingCreds) RequireTransportSecurity() bool {
	return true
}

// A representation of v1 Config entry.
type legacyConfig struct {
	ConfigSet   string `json:"config_set"`
	Content     []byte `json:"content"`
	ContentHash string `json:"content_hash"`
	Revision    string `json:"revision"`
	URL         string `json:"url"`
}

// getProjectConfigs calls the LUCI Config v2 method, forwarding credentials.
func getProjectConfigs(ctx context.Context, client config.Interface, path string, req http.Header) ([]legacyConfig, error) {
	// Put caller's credentials into the request context, where forwardingCreds
	// looks for them.
	var creds map[string]string
	if auth := req.Get("Authorization"); auth != "" {
		creds = map[string]string{"authorization": auth}
	}
	ctx = context.WithValue(ctx, &forwardedCredsCtxKey, creds)

	cfgs, err := client.GetProjectConfigs(ctx, path, false)
	if err != nil {
		return nil, err
	}

	legacy := make([]legacyConfig, len(cfgs))
	for i, cfg := range cfgs {
		legacy[i] = legacyConfig{
			ConfigSet:   string(cfg.ConfigSet),
			Content:     []byte(cfg.Content),
			ContentHash: cfg.ContentHash,
			Revision:    cfg.Revision,
			URL:         cfg.ViewURL,
		}
	}
	return legacy, nil
}

const frontendRedirectHTML = `<html><head><script>
(function() {
	const destHost = %s;

	const projectsPfx = "#/projects/";
	const servicesPfx = "#/services/";

	let configSet = "";
	let hash = window.location.hash;
	if (hash.startsWith(projectsPfx) || hash.startsWith(servicesPfx)) {
		configSet = hash.slice(2);
	} else if (hash.startsWith("#/q/")) {
		let reminder = hash.slice(4);
		if (reminder != "") {
			configSet = "services/" + reminder;
		}
	}

	if (configSet != "") {
		window.location.replace("https://" + destHost + "/config_set/" + configSet);
	} else {
		window.location.replace("https://" + destHost);
	}
})()
</script></head></html>
`

const discoveryDoc = `{
  "auth": {
    "oauth2": {
      "scopes": {
        "https://www.googleapis.com/auth/userinfo.email": {
          "description": "https://www.googleapis.com/auth/userinfo.email"
        }
      }
    }
  },
  "basePath": "/_ah/api/config/v1",
  "baseUrl": "%s/_ah/api/config/v1",
  "batchPath": "batch",
  "description": "API to access configurations.",
  "discoveryVersion": "v1",
  "icons": {
    "x16": "https://www.google.com/images/icons/product/search-16.gif",
    "x32": "https://www.google.com/images/icons/product/search-32.gif"
  },
  "id": "config:v1",
  "kind": "discovery#restDescription",
  "methods": {
    "get_project_configs": {
      "description": "Gets configs in all project config sets.",
      "httpMethod": "GET",
      "id": "config.get_project_configs",
      "parameterOrder": [
        "path"
      ],
      "parameters": {
        "hashes_only": {
          "location": "query",
          "type": "boolean"
        },
        "path": {
          "location": "path",
          "required": true,
          "type": "string"
        }
      },
      "path": "configs/projects/{path}",
      "response": {
        "$ref": "LuciConfigGetConfigMultiResponseMessage"
      },
      "scopes": [
        "https://www.googleapis.com/auth/userinfo.email"
      ]
    }
  },
  "name": "config",
  "parameters": {
    "alt": {
      "default": "json",
      "description": "Data format for the response.",
      "enum": [
        "json"
      ],
      "enumDescriptions": [
        "Responses with Content-Type of application/json"
      ],
      "location": "query",
      "type": "string"
    },
    "fields": {
      "description": "Selector specifying which fields to include in a partial response.",
      "location": "query",
      "type": "string"
    },
    "key": {
      "description": "API key. Your API key identifies your project and provides you with API access, quota, and reports. Required unless you provide an OAuth 2.0 token.",
      "location": "query",
      "type": "string"
    },
    "oauth_token": {
      "description": "OAuth 2.0 token for the current user.",
      "location": "query",
      "type": "string"
    },
    "prettyPrint": {
      "default": "true",
      "description": "Returns response with indentations and line breaks.",
      "location": "query",
      "type": "boolean"
    },
    "quotaUser": {
      "description": "Available to use for quota purposes for server-side applications. Can be any arbitrary string assigned to a user, but should not exceed 40 characters. Overrides userIp if both are provided.",
      "location": "query",
      "type": "string"
    },
    "userIp": {
      "description": "IP address of the site where the request originates. Use this if you want to enforce per-user limits.",
      "location": "query",
      "type": "string"
    }
  },
  "protocol": "rest",
  "rootUrl": "%s/_ah/api/",
  "schemas": {
    "LuciConfigGetConfigMultiResponseMessage": {
      "id": "LuciConfigGetConfigMultiResponseMessage",
      "properties": {
        "configs": {
          "items": {
            "$ref": "LuciConfigGetConfigMultiResponseMessageConfigEntry"
          },
          "type": "array"
        }
      },
      "type": "object"
    },
    "LuciConfigGetConfigMultiResponseMessageConfigEntry": {
      "id": "LuciConfigGetConfigMultiResponseMessageConfigEntry",
      "properties": {
        "config_set": {
          "required": true,
          "type": "string"
        },
        "content": {
          "format": "byte",
          "type": "string"
        },
        "content_hash": {
          "required": true,
          "type": "string"
        },
        "revision": {
          "required": true,
          "type": "string"
        },
        "url": {
          "type": "string"
        }
      },
      "type": "object"
    }
  },
  "servicePath": "config/v1/",
  "title": "Configuration Service (shim)",
  "version": "v1"
}`
