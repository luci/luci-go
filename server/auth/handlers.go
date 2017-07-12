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

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth/signing"
	"github.com/luci/luci-go/server/router"
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
}

// certsHandler servers public certificates of the signer in the context.
func certsHandler(c *router.Context) {
	s := getSigner(c.Context)
	if s == nil {
		httpReplyError(c, http.StatusNotFound, "No Signer instance available")
		return
	}
	certs, err := s.Certificates(c.Context)
	if err != nil {
		httpReplyError(c, http.StatusInternalServerError, fmt.Sprintf("Can't fetch certificates - %s", err))
	} else {
		httpReply(c, http.StatusOK, certs)
	}
}

// infoHandler returns information about the current service identity.
func infoHandler(c *router.Context) {
	s := getSigner(c.Context)
	if s == nil {
		httpReplyError(c, http.StatusNotFound, "No Signer instance available")
		return
	}
	info, err := s.ServiceInfo(c.Context)
	if err != nil {
		httpReplyError(c, http.StatusInternalServerError, fmt.Sprintf("Can't grab service info - %s", err))
	} else {
		httpReply(c, http.StatusOK, info)
	}
}

////

func getSigner(c context.Context) signing.Signer {
	if cfg := getConfig(c); cfg != nil {
		return cfg.Signer
	}
	return nil
}

func httpReply(c *router.Context, code int, out interface{}) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(code)
	if err := json.NewEncoder(c.Writer).Encode(out); err != nil {
		logging.Errorf(c.Context, "Failed to JSON encode output - %s", err)
	}
}

func httpReplyError(c *router.Context, code int, msg string) {
	errorReply := struct {
		Error string `json:"error"`
	}{msg}
	httpReply(c, code, &errorReply)
}
