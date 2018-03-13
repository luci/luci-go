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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

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

// InstallHandlers installs the metadata and validation handlers that use
// the given validation rules.
//
// It does not implement any authentication checks, thus the passed in
// router.MiddlewareChain should implement any necessary authentication checks.
func InstallHandlers(r *router.Router, base router.MiddlewareChain, rules *RuleSet) {
	r.GET(metadataPath, base, metadataRequestHandler(rules))
	r.POST(validationPath, base, validationRequestHandler(rules))
}

func badRequestStatus(c context.Context, w http.ResponseWriter, msg string, err error) {
	if err != nil {
		logging.WithError(err).Warningf(c, "%s", msg)
	} else {
		logging.Warningf(c, "%s", msg)
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(msg))
}

func internalErrStatus(c context.Context, w http.ResponseWriter, msg string, err error) {
	logging.WithError(err).Errorf(c, "%s", msg)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(msg))
}

// validationRequestHandler handles the validation request from luci-config and
// responds with the corresponding results.
func validationRequestHandler(rules *RuleSet) router.Handler {
	return func(ctx *router.Context) {
		c, w, r := ctx.Context, ctx.Writer, ctx.Request

		var reqBody config.ValidationRequestMessage
		switch err := json.NewDecoder(r.Body).Decode(&reqBody); {
		case err != nil:
			badRequestStatus(c, w, "Validation: error decoding request body", err)
			return
		case reqBody.GetConfigSet() == "":
			badRequestStatus(c, w, "Must specify the config_set of the file to validate", nil)
			return
		case reqBody.GetPath() == "":
			badRequestStatus(c, w, "Must specify the path of the file to validate", nil)
			return
		}

		vc := &Context{Context: c}
		vc.SetFile(reqBody.GetPath())
		err := rules.ValidateConfig(vc, reqBody.GetConfigSet(), reqBody.GetPath(), reqBody.GetContent())
		if err != nil {
			internalErrStatus(c, w, "Validation: transient failure", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		var msgList []*config.ValidationResponseMessage_Message
		if len(vc.errors) == 0 {
			logging.Infof(c, "No validation errors")
		} else {
			var errorBuffer bytes.Buffer
			for _, error := range vc.errors {
				// validation.Context currently only supports ERROR severity
				err := error.Error()
				msgList = append(msgList, &config.ValidationResponseMessage_Message{
					Severity: config.ValidationResponseMessage_ERROR.Enum(),
					Text:     &err,
				})
				errorBuffer.WriteString("\n  " + err)
			}
			logging.Warningf(c, "Validation errors%s", errorBuffer.String())
		}
		if err := json.NewEncoder(w).Encode(config.ValidationResponseMessage{Messages: msgList}); err != nil {
			internalErrStatus(c, w, "Validation: failed to JSON encode output", err)
		}
	}
}

// metadataRequestHandler handles the metadata request from luci-config and
// responds with the necessary metadata defined by the given Validator.
func metadataRequestHandler(rules *RuleSet) router.Handler {
	return func(ctx *router.Context) {
		c, w := ctx.Context, ctx.Writer

		patterns, err := rules.ConfigPatterns(c)
		if err != nil {
			internalErrStatus(c, w, "Metadata: failed to collect the list of validation patterns", err)
			return
		}

		meta := config.ServiceDynamicMetadata{
			Version: proto.String(metaDataFormatVersion),
			Validation: &config.Validator{
				Url: proto.String(fmt.Sprintf("https://%s%s", ctx.Request.URL.Host, validationPath)),
			},
		}
		for _, p := range patterns {
			meta.Validation.Patterns = append(meta.Validation.Patterns, &config.ConfigPattern{
				ConfigSet: proto.String(p.ConfigSet.String()),
				Path:      proto.String(p.Path.String()),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(&meta); err != nil {
			internalErrStatus(c, w, "Metadata: failed to JSON encode output", err)
		}
	}
}
