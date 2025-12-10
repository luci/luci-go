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

package androidbuild

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"

	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Config represents a compiled android build configuration.
type Config struct {
	DataRealmPattern *regexp.Regexp
	DataRealms       map[string]*configpb.AndroidBuild_ByDataRealmConfig
}

// Default configuration to use if no service configuration is present.
var DefaultConfig = &configpb.AndroidBuild{
	DataRealmPattern: "^prod$",
	DataRealms:       map[string]*configpb.AndroidBuild_ByDataRealmConfig{},
}

// NewProducerSystem compiles a Config object from the configuration.
func NewConfig(cfg *configpb.AndroidBuild) (*Config, error) {
	if cfg == nil {
		// If configuration is not present, use DefaultConfig.
		cfg = DefaultConfig
	}

	dataRealmPattern, err := regexp.Compile(cfg.DataRealmPattern)
	if err != nil {
		return nil, errors.Fmt("data_realm_pattern: %w", err)
	}
	return &Config{
		DataRealmPattern: dataRealmPattern,
		DataRealms:       cfg.DataRealms,
	}, nil
}

// ValidateBuildDescriptor validates a AndroidBuildDescriptor against the android build config.
func (c *Config) ValidateBuildDescriptor(resource *pb.AndroidBuildDescriptor) error {
	if !c.DataRealmPattern.MatchString(resource.DataRealm) {
		return errors.Fmt("data_realm: does not match pattern %q", c.DataRealmPattern.String())
	}
	return nil
}

// ValidateBuildDescriptor validates a SubmittedAndroidBuild against the android build config.
func (c *Config) ValidateSubmittedBuild(resource *pb.SubmittedAndroidBuild) error {
	if !c.DataRealmPattern.MatchString(resource.DataRealm) {
		return errors.Fmt("data_realm: does not match pattern %q", c.DataRealmPattern.String())
	}
	return nil
}

// GenerateURL generates a URL for the build descriptor.
func (c *Config) GenerateBuildDescriptorURL(resource *pb.AndroidBuildDescriptor) (string, error) {
	if err := c.ValidateBuildDescriptor(resource); err != nil {
		return "", err
	}

	cfg := c.DataRealms[resource.DataRealm]
	if cfg == nil {
		return "", nil
	}
	template := cfg.FullBuildUrlTemplate
	if template == "" {
		return "", nil
	}

	variables := make(map[string]string)
	variables["branch"] = resource.Branch
	variables["build_target"] = resource.BuildTarget
	variables["build_id"] = resource.BuildId

	// Replace variables in template.
	url := template
	for name, value := range variables {
		url = strings.ReplaceAll(url, fmt.Sprintf("${%s}", name), value)
	}
	return url, nil
}
