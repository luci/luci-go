// Copyright 2015 The LUCI Authors.
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

package auth

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/router"
)

// InstallHandlers installs authentication related HTTP handlers.
//
// All new HTTP routes live under '/auth/api/' prefix.
//
// If you are using appengine/gaeauth/server, these handlers are already
// installed.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET("/auth/api/v1/server/certificates", base, certsHandler)
	r.GET("/auth/api/v1/server/info", base, infoHandler)
	r.GET("/auth/api/v1/server/client_id", base, clientIDHandler)
}

// certsHandler servers public certificates of the signer in the context.
func certsHandler(c *router.Context) {
	s := GetSigner(c.Request.Context())
	if s == nil {
		httpReplyError(c, http.StatusNotFound, "No Signer instance available")
		return
	}
	certs, err := s.Certificates(c.Request.Context())
	if err != nil {
		httpReplyError(c, http.StatusInternalServerError, fmt.Sprintf("Can't fetch certificates - %s", err))
	} else {
		httpReply(c, http.StatusOK, certs)
	}
}

// infoHandler returns information about the current service identity.
func infoHandler(c *router.Context) {
	s := GetSigner(c.Request.Context())
	if s == nil {
		httpReplyError(c, http.StatusNotFound, "No Signer instance available")
		return
	}
	info, err := s.ServiceInfo(c.Request.Context())
	if err != nil {
		httpReplyError(c, http.StatusInternalServerError, fmt.Sprintf("Can't grab service info - %s", err))
	} else {
		httpReply(c, http.StatusOK, info)
	}
}

// clientIDHandler returns OAuth2.0 client ID intended for the frontend.
func clientIDHandler(c *router.Context) {
	clientID, err := GetFrontendClientID(c.Request.Context())
	if err != nil {
		httpReplyError(c, http.StatusInternalServerError, fmt.Sprintf("Can't grab the client ID - %s", err))
	} else {
		httpReply(c, http.StatusOK, map[string]string{"client_id": clientID})
	}
}

////

func httpReply(c *router.Context, code int, out any) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(code)
	if err := json.NewEncoder(c.Writer).Encode(out); err != nil {
		logging.Errorf(c.Request.Context(), "Failed to JSON encode output - %s", err)
	}
}

func httpReplyError(c *router.Context, code int, msg string) {
	errorReply := struct {
		Error string `json:"error"`
	}{msg}
	httpReply(c, code, &errorReply)
}
