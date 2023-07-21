// Copyright 2023 The LUCI Authors.
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

package service

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
)

func validateMetadata(metadata *cfgcommonpb.ServiceMetadata) error {
	var errs []error
	for i, pattern := range metadata.GetConfigPatterns() {
		if err := validateConfigPattern(pattern); err != nil {
			errs = append(errs, fmt.Errorf("invalid config pattern [%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func validateLegacyMetadata(legacyMetadata *cfgcommonpb.ServiceDynamicMetadata) error {
	var errs []error
	for i, pattern := range legacyMetadata.GetValidation().GetPatterns() {
		if err := validateConfigPattern(pattern); err != nil {
			errs = append(errs, fmt.Errorf("invalid config pattern [%d]: %w", i, err))
		}
	}
	switch u := legacyMetadata.GetValidation().GetUrl(); {
	case u == "":
		errs = append(errs, errors.New("empty validation url"))
	default:
		if _, err := url.Parse(u); err != nil {
			errs = append(errs, fmt.Errorf("invalid url %q: %w", u, err))
		}
	}
	return errors.Join(errs...)
}

func validateConfigPattern(pattern *cfgcommonpb.ConfigPattern) error {
	if expr, found := strings.CutPrefix(pattern.GetConfigSet(), "regex:"); found {
		if _, err := regexp.Compile(expr); err != nil {
			return fmt.Errorf("invalid regular expression %q for config set pattern: %w", expr, err)
		}
	}
	if expr, found := strings.CutPrefix(pattern.GetPath(), "regex:"); found {
		if _, err := regexp.Compile(expr); err != nil {
			return fmt.Errorf("invalid regular expression %q for path pattern: %w", expr, err)
		}
	}
	return nil
}
