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
	"net/http"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// Taken from
// https://chromium.googlesource.com/infra/luci/luci-py/+/3efc60daef6bf6669f9211f63e799db47a0478c0/appengine/components/components/config/endpoint.py
const (
	metaDataFormatVersion = "1.0"
)

type configPattern struct {
	configSetPattern string
	pathPattern      string
}

type valMetaImpl struct {
	url      string
	patterns []*configPattern
}

type metaImpl struct {
	validation *valMetaImpl
	version    string
}

// Metadata for config validation.
type Metadata interface {
	// AddConfigPattern adds the regex pattern pair to the existing list of pattern pairs
	AddConfigPattern(configSetPattern string, pathPattern string) error

	// GetMetadataHandler implements the handler for GET requests for metadata
	GetMetadataHandler(ctx *router.Context)
}

// AddConfigPattern implements Metadata interface.
func (metaImpl *metaImpl) AddConfigPattern(configSetPattern string, pathPattern string) error {
	// use pattern.Parse to verify the configSetPattern
	_, err := pattern.Parse(configSetPattern)
	if err != nil {
		return err
	}
	_, err = pattern.Parse(pathPattern)
	if err != nil {
		return err
	}
	cp := &configPattern{configSetPattern: configSetPattern, pathPattern: pathPattern}
	metaImpl.validation.patterns = append(metaImpl.validation.patterns, cp)
	return nil
}

// GetMetadataHandler is the handler for the endpoint
func (metaImpl *metaImpl) GetMetadataHandler(ctx *router.Context) {
	c, w := ctx.Context, ctx.Writer
	if err := json.NewEncoder(w).Encode(metaImpl); err != nil {
		logging.Errorf(c, "Metadata: failed to JSON encode output - %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// GetMetadata returns the metadata with fields set as the arguments as default.
func GetMetadata(validationURL string, cs string, csPath string) (Metadata, error) {
	// cs and path are added to the list of patterns by default
	_, err := pattern.Parse(cs)
	if err != nil {
		return nil, err
	}
	_, err = pattern.Parse(csPath)
	if err != nil {
		return nil, err
	}
	cp := &configPattern{configSetPattern: cs, pathPattern: csPath}
	valMeta := &valMetaImpl{
		url:      validationURL,
		patterns: []*configPattern{cp}}
	return &metaImpl{validation: valMeta, version: metaDataFormatVersion}, nil
}
