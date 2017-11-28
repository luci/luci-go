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
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/router"
)

const (
	// paths for handlers
	metadataPath   = "/api/config/v1/metadata"
	validationPath = "/api/config/v1/validate"

	// Taken from
	// https://chromium.googlesource.com/infra/luci/luci-py/+/3efc60daef6bf6669f9211f63e799db47a0478c0/appengine/components/components/config/endpoint.py
	metaDataFormatVersion = "1.0"
)

// ConfigPattern is a pair of pattern.Pattern of configSets and paths that
// the importing service is responsible for validating.
type ConfigPattern struct {
	ConfigSet pattern.Pattern
	Path      pattern.Pattern
}

// Validator defines the components needed to install handlers to
// implement config validation protocol.
type Validator struct {
	// ConfigPatterns is the list of patterns of configSets and paths that the
	// service is responsible for validating.
	ConfigPatterns []*ConfigPattern

	// Func performs the actual config validation and stores the
	// associated results in the validation.Context. content is a base-64
	// encoded string while configSet and path are not.
	Func func(configSet, path, content string, ctx *Context)
}

// InstallHandlers installs the metadata and validation handlers as defined by
// the given Validator on the given router. It does not implement any
// authentication checks, thus the passed in router.MiddlewareChain should
// implement any necessary authentication checks.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, validator *Validator) {
	r.GET(metadataPath, base, validator.metadataRequestHandler)
	r.POST(validationPath, base, validator.validationRequestHandler)
}

func errStatus(c context.Context, w http.ResponseWriter, status int, msg string) {
	logging.Errorf(c, msg)
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// validationRequestHandler handles the validation request from luci-config and
// responds with the corresponding results.
func (validator *Validator) validationRequestHandler(ctx *router.Context) {
	c, w, r := ctx.Context, ctx.Writer, ctx.Request
	var reqBody config.ValidationRequestMessage
	switch err := json.NewDecoder(r.Body).Decode(&reqBody); {
	case err != nil:
		logging.WithError(err).Errorf(c, "validation: error decoding request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	case reqBody.GetConfigSet() == "":
		errStatus(c, w, http.StatusBadRequest, "Must specify the config_set of the file to validate")
	case reqBody.GetPath() == "":
		errStatus(c, w, http.StatusBadRequest, "Must specify the path of the file to validate")
	}

	vc := &Context{}
	validator.Func(reqBody.GetConfigSet(), reqBody.GetPath(), reqBody.GetContent(), vc)
	w.Header().Set("Content-Type", "application/json")
	var errList []map[string]string
	for _, error := range vc.errors {
		// validation.Context currently only supports ERROR severity
		errList = append(errList, map[string]string{"severity": "ERROR", "text": error.Error()})
	}
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"messages": errList}); err != nil {
		logging.WithError(err).Errorf(c, "Validation: failed to JSON encode output")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// metadataRequestHandler handles the metadata request from luci-config and
// responds with the necessary metadata defined by the given Validator.
func (validator *Validator) metadataRequestHandler(ctx *router.Context) {
	c, w := ctx.Context, ctx.Writer
	var patterns []*config.ConfigPattern
	for _, p := range validator.ConfigPatterns {
		css := p.ConfigSet.String()
		ps := p.Path.String()
		patterns = append(patterns, &config.ConfigPattern{ConfigSet: &css, Path: &ps})
	}
	url := fmt.Sprintf("https://%s%s", ctx.Request.URL.Host, metadataPath)
	val := config.Validator{Url: &url, Patterns: patterns}
	version := metaDataFormatVersion
	if err := json.NewEncoder(w).Encode(config.ServiceDynamicMetadata{Version: &version, Validation: &val}); err != nil {
		logging.WithError(err).Errorf(c, "Metadata: failed to JSON encode output")
		w.WriteHeader(http.StatusInternalServerError)
	}
}
