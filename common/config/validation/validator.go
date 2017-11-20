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

// Package validation provides helpers for performing and setting up handlers for config validation related requests
// from luci-config.
package validation

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

const (
	// auth related
	configAdmins    = "config-admins"
	luciConfigEmail = "luci-config@appspot.gserviceaccount.com"

	// paths for handlers
	metadataPath   = "api/config/v1/metadata"
	validationPath = "api/config/v1/validate"

	// Taken from
	// https://chromium.googlesource.com/infra/luci/luci-py/+/3efc60daef6bf6669f9211f63e799db47a0478c0/appengine/components/components/config/endpoint.py
	metaDataFormatVersion = "1.0"
)

// ConfigPattern is the pair of pattern.Pattern of configSets and paths that the importing service is
// responsible for validating.
type ConfigPattern struct {
	ConfigSet pattern.Pattern
	Path      pattern.Pattern
}

// ConfigValidator defines the components needed to install handlers to handle GET/POST requests for
// metadata and config validation.
type ConfigValidator struct {
	// ConfigPatterns is the list of patterns of configSets and paths that the service is responsible for validating.
	ConfigPatterns []*ConfigPattern

	// Validator is the function that takes configSet, path, content and validation.Context as its arguments,
	// performs the actual config validation and stores the associated results in the validation.Context.
	Validator func(configSet string, path string, content string, ctx *Context)
}

// InstallHandlers installs the metadata and validation request handlers as defined by
// the given ConfigValidator on the given router.
func InstallHandlers(configValidator *ConfigValidator, r *router.Router) {
	a := auth.Authenticator{
		Methods: []auth.Method{
			&server.OAuth2Method{Scopes: []string{server.EmailScope}},
			&server.InboundAppIDAuthMethod{},
		},
	}
	authmw := standard.Base().Extend(a.GetMiddleware()).Extend(checkAuth)
	r.GET(fmt.Sprintf("/%s", metadataPath), authmw, configValidator.metadataRequestHandler)
	r.POST(fmt.Sprintf("/%s", validationPath), authmw, configValidator.validationRequestHandler)
}

func errStatus(c context.Context, w http.ResponseWriter, status int, msg string) {
	logging.Errorf(c, "Status %d msg %s", status, msg)
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// Checks whether the requester is luci-config or in the config-admins group.
func checkAuth(c *router.Context, next router.Handler) {
	isConfigAdmin, err := auth.IsMember(c.Context, configAdmins)
	if err != nil {
		errStatus(c.Context, c.Writer, http.StatusInternalServerError, "Could not authenticate")
	}
	if auth.CurrentIdentity(c.Context).Email() == luciConfigEmail || isConfigAdmin {
		next(c)
	} else {
		errStatus(c.Context, c.Writer, http.StatusForbidden, "Not authorized")
	}
}

// validationRequestHandler handles the validation request from luci-config and responds with the corresponding results.
func (configValidator *ConfigValidator) validationRequestHandler(ctx *router.Context) {
	c, w, r := ctx.Context, ctx.Writer, ctx.Request

	valRequestBody := struct {
		ConfigSet string `json:"config_set"`
		Path      string `json:"path"`
		Content   string `json:"content"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&valRequestBody); err != nil {
		logging.WithError(err).Errorf(c, "validation: error decoding request body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if valRequestBody.ConfigSet == "" {
		errStatus(c, w, http.StatusBadRequest, "Must specify the config_set of the file to validate")
		return
	}
	if valRequestBody.Path == "" {
		errStatus(c, w, http.StatusBadRequest, "Must specify the path of the file to validate")
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(valRequestBody.Content)
	if err != nil {
		logging.WithError(err).Errorf(c, "validation: error in base64 decoding config file content")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	valCtx := &Context{}
	configValidator.Validator(valRequestBody.ConfigSet, valRequestBody.Path, string(decoded), valCtx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(constructResponseFromContext(valCtx)); err != nil {
		logging.Errorf(c, "Validation: failed to JSON encode output - %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

type validationResponseMessage struct {
	Messages []*validationErrorMessage `json:"messages"`
}

type validationErrorMessage struct {
	Severity string `json:"severity"`
	Text     string `json:"text"`
}

func constructResponseFromContext(valCtx *Context) *validationResponseMessage {
	valRespMsg := &validationResponseMessage{}
	for _, error := range valCtx.errors {
		// Assumes all the errors are the ERROR level for now
		valRespMsg.Messages = append(
			valRespMsg.Messages, &validationErrorMessage{Severity: "ERROR", Text: error.Error()})
	}
	return valRespMsg
}

type patternStringPair struct {
	ConfigSet string `json:"config_set"`
	Path      string `json:"path"`
}

type validationMetadata struct {
	URL      string               `json:"url"`
	Patterns []*patternStringPair `json:"patterns"`
}

type metadata struct {
	Validation *validationMetadata `json:"validation"`
	Version    string              `json:"version"`
}

// metadataRequestHandler handles the metadata request from luci-config and responds with the necessary metadata
// defined by the given ConfigValidator.
func (configValidator *ConfigValidator) metadataRequestHandler(ctx *router.Context) {
	c, w := ctx.Context, ctx.Writer
	if _, ok := ctx.Request.Header["Host"]; !ok {
		logging.Errorf(c, "Metadata: host URL not found in http request header")
		w.WriteHeader(http.StatusInternalServerError)
	}
	valMeta := &validationMetadata{URL: fmt.Sprintf("https://%s/%s", ctx.Request.Header["Host"], metadataPath)}
	for _, configPattern := range configValidator.ConfigPatterns {
		patternPair := &patternStringPair{
			ConfigSet: configPattern.ConfigSet.String(),
			Path:      configPattern.Path.String()}
		valMeta.Patterns = append(valMeta.Patterns, patternPair)
	}
	meta := &metadata{Validation: valMeta, Version: metaDataFormatVersion}
	if err := json.NewEncoder(w).Encode(meta); err != nil {
		logging.Errorf(c, "Metadata: failed to JSON encode output - %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
