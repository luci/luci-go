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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

type valImpl struct {
	// Mapping of the cfg_set and path to the validator function.
	validatorFunction ValidatorFunction
}

// ValidatorFunction is the function that performs the actual config validation.
type ValidatorFunction func(cs string, path string, content string, ctx *Context)

// Validator interface to handle the validation.
type Validator interface {
	// Handler
	ValidationHandler(ctx *router.Context)
}

type responseMessage struct {
	messages []*errorResponseMessage
}

type errorResponseMessage struct {
	severity string
	text  string
}

// ValidationHandler handles the validation request and sends the validation results back.
func (valImpl *valImpl) ValidationHandler(ctx *router.Context) {
	c, w, r := ctx.Context, ctx.Writer, ctx.Request

	validationRequestBody := struct {
		ConfigSet string `json:"config_set"`
		Path      string `json:"path"`
		Content   string `json:"content"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&validationRequestBody); err != nil {
		logging.WithError(err).Errorf(c, "validation: error decoding request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cs := validationRequestBody.ConfigSet
	path := validationRequestBody.Path
	content, err := base64.StdEncoding.DecodeString(validationRequestBody.Content)
	if err != nil {
		logging.WithError(err).Errorf(c, "validation: error base64 decoding request content")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	valCtx := &Context{}
	valImpl.validatorFunction(cs, path, string(content), valCtx)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(constructResponseFromContext(valCtx)); err != nil {
		logging.Errorf(c, "Validation: failed to JSON encode output - %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// GetValidatorFromFunction returns a Validator interface given a function that performs the actual validation.
func GetValidatorFromFunction(validatorFunction ValidatorFunction) Validator {
	return &valImpl{
		validatorFunction: validatorFunction,
	}
}

// Assume all the errors are the ERROR level for now
func constructResponseFromContext(valCtx *Context) *responseMessage {
	valRespMsg := &responseMessage{}
	for _, error := range valCtx.errors {
		valRespMsg.messages = append(
			valRespMsg.messages, &errorResponseMessage{severity: "ERROR", text: error.Error()})
	}
	return valRespMsg
}
