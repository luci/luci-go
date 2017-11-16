// Copyright 2017 The LUCI Authors.
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

package validation

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

const (
	configAdmins   = "config-admins"
	luciConfigName = "luci-config@appspot.gserviceaccount.com"
)

func errStatus(c context.Context, w http.ResponseWriter, status int, msg string) {
	logging.Errorf(c, "Status %d msg %s", status, msg)
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// Checks whether the requester is luci-config or in the config-admins group
func checkAuth(c *router.Context, next router.Handler) {
	isConfigAdmin, err := auth.IsMember(c.Context, configAdmins)
	if err != nil {
		errStatus(c.Context, c.Writer, http.StatusForbidden, "Could not authenticate requester")
	}
	if auth.CurrentIdentity(c.Context).Email() == luciConfigName || isConfigAdmin {
		next(c)
	} else {
		errStatus(c.Context, c.Writer, http.StatusForbidden, "Access denied")
	}
}

func base() router.MiddlewareChain {
	a := auth.Authenticator{
		Methods: []auth.Method{
			&server.OAuth2Method{Scopes: []string{server.EmailScope}},
			&server.InboundAppIDAuthMethod{},
		},
	}
	return standard.Base().Extend(a.GetMiddleware())
}

// InstallMetadataAndValidationHandlers sets up the handlers related to metadata and validation.
// Given the Router, Validator and Metadata it installs handlers for GET requests for Metadata and POST requests
// for validating config files and their contents from luci-config.
func InstallMetadataAndValidationHandlers(validator Validator, metadata Metadata, r *router.Router) {
	authmw := base().Extend(checkAuth)

	r.GET("/api/config/v1/metadata", authmw, metadata.MetadataRequestHandler)
	r.POST("/api/config/v1/validate", authmw, validator.ValidationRequestHandler)
}

// MetadataRequestHandler provides the handler for GET requests for metadata.
func (metadata *Metadata) MetadataRequestHandler(ctx *router.Context) {
	c, w := ctx.Context, ctx.Writer
	if err := json.NewEncoder(w).Encode(metadata); err != nil {
		logging.Errorf(c, "Metadata: failed to JSON encode output - %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// ValidationRequestHandler handles the validation request from luci-config and sends the corresponding results back.
func (validator Validator) ValidationRequestHandler(ctx *router.Context) {
	c, w, r := ctx.Context, ctx.Writer, ctx.Request

	valRequestBody := struct {
		ConfigSet string `json:"config_set"`
		Path      string `json:"path"`
		Content   string `json:"content"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&valRequestBody); err != nil {
		logging.WithError(err).Errorf(c, "validation: error decoding request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(valRequestBody.Content)
	if err != nil {
		logging.WithError(err).Errorf(c, "validation: error base64 decoding config file content")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	valCtx := &Context{}
	validator(valRequestBody.ConfigSet, valRequestBody.Path, string(decoded), valCtx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(constructResponseFromContext(valCtx)); err != nil {
		logging.Errorf(c, "Validation: failed to JSON encode output - %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// Assumes all the errors are the ERROR level for now
func constructResponseFromContext(valCtx *Context) *responseMessage {
	valRespMsg := &responseMessage{}
	for _, error := range valCtx.errors {
		valRespMsg.messages = append(
			valRespMsg.messages, &errorResponseMessage{Severity: "ERROR", Text: error.Error()})
	}
	return valRespMsg
}
