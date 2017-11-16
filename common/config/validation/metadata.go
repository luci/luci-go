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
	"go.chromium.org/luci/common/data/text/pattern"
)

// Taken from
// https://chromium.googlesource.com/infra/luci/luci-py/+/3efc60daef6bf6669f9211f63e799db47a0478c0/appengine/components/components/config/endpoint.py
const (
	metaDataFormatVersion = "1.0"
)

type configPattern struct {
	ConfigSetPattern string `json:"config_set"`
	PathPattern      string `json:"path"`
}

type validatorMetadata struct {
	URL      string           `json:"url"`
	Patterns []*configPattern `json:"patterns"`
}

// Metadata stores the values with which to respond to GET requests from luci-config for metadata.
type Metadata struct {
	Validation *validatorMetadata `json:"validation"`
	Version    string             `json:"version"`
}

// AddConfigPattern takes a regex pattern pair and adds it to the already existing list of pattern pairs if the regex
// pairs are valid.
func (metadata *Metadata) AddConfigPattern(configSetPattern string, pathPattern string) error {
	_, err := pattern.Parse(configSetPattern)
	if err != nil {
		return err
	}
	_, err = pattern.Parse(pathPattern)
	if err != nil {
		return err
	}
	cp := &configPattern{ConfigSetPattern: configSetPattern, PathPattern: pathPattern}
	metadata.Validation.Patterns = append(metadata.Validation.Patterns, cp)
	return nil
}

// ConstructMetadata constructs the metadata struct with the fields set as the arguments given.
// configSet and and configSetPath are added to the set of patterns by default.
// If configSet and configSetPath are not valid patterns, an error is returned.
func ConstructMetadata(validationURL string, configSet string, configSetPath string) (*Metadata, error) {
	valMeta := &validatorMetadata{
		Url:      validationURL,
		Patterns: []*configPattern{}}
	meta := &Metadata{Validation: valMeta, Version: metaDataFormatVersion}
	if err := meta.AddConfigPattern(configSet, configSetPath); err != nil {
		return nil, err
	}
	return meta, nil
}
