// Copyright 2025 The LUCI Authors.
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

package producersystems

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"

	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ProducerSystem represents a compiled producer system configuration.
type ProducerSystem struct {
	System                 string
	NamePattern            *regexp.Regexp
	DataRealmPattern       *regexp.Regexp
	ValidateCallers        bool
	URLTemplate            string
	URLTemplateByDataRealm map[string]string
}

// NewProducerSystem compiles a ProducerSystem from the configuration.
func NewProducerSystem(cfg *configpb.ProducerSystem) (*ProducerSystem, error) {
	namePattern, err := regexp.Compile(cfg.NamePattern)
	if err != nil {
		return nil, errors.Fmt("name_pattern: %w", err)
	}
	dataRealmPattern, err := regexp.Compile(cfg.DataRealmPattern)
	if err != nil {
		return nil, errors.Fmt("data_realm_pattern: %w", err)
	}
	return &ProducerSystem{
		System:                 cfg.System,
		NamePattern:            namePattern,
		DataRealmPattern:       dataRealmPattern,
		ValidateCallers:        cfg.ValidateCallers,
		URLTemplate:            cfg.UrlTemplate,
		URLTemplateByDataRealm: cfg.UrlTemplateByDataRealm,
	}, nil
}

// Validate validates a ProducerResource against the producer system.
func (ps *ProducerSystem) Validate(resource *pb.ProducerResource) error {
	if resource.System != ps.System {
		return errors.Fmt("system: expected %q, got %q", ps.System, resource.System)
	}
	if !ps.NamePattern.MatchString(resource.Name) {
		return errors.Fmt("name: does not match pattern %q", ps.NamePattern.String())
	}
	if !ps.DataRealmPattern.MatchString(resource.DataRealm) {
		return errors.Fmt("data_realm: does not match pattern %q", ps.DataRealmPattern.String())
	}
	return nil
}

// GenerateURL generates a URL for the producer resource.
func (ps *ProducerSystem) GenerateURL(resource *pb.ProducerResource) (string, error) {
	if err := ps.Validate(resource); err != nil {
		return "", err
	}

	template := ps.URLTemplate
	if t, ok := ps.URLTemplateByDataRealm[resource.DataRealm]; ok {
		template = t
	}
	if template == "" {
		return "", nil
	}

	// Extract variables from name.
	matches := ps.NamePattern.FindStringSubmatch(resource.Name)
	if matches == nil {
		// Should have been caught by Validate, but check anyway.
		return "", errors.Fmt("name does not match pattern %q", ps.NamePattern.String())
	}
	variables := make(map[string]string)
	for i, name := range ps.NamePattern.SubexpNames() {
		if i != 0 && name != "" {
			variables[name] = matches[i]
		}
	}
	variables["data_realm"] = resource.DataRealm

	// Replace variables in template.
	url := template
	for name, value := range variables {
		url = strings.ReplaceAll(url, fmt.Sprintf("${%s}", name), value)
	}
	return url, nil
}
