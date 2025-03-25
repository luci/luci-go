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

// Package schemes contains the implementation of ResultDB Schemes, a way
// of defining additional validation constraints for a type of test.
package schemes

import (
	"regexp"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Scheme represents the configuration for a type of test.
// For example, JUnit or GTest.
type Scheme struct {
	// The identifier of the scheme.
	ID string
	// The human readable scheme name.
	HumanReadableName string
	// Configuration for the coarse name level.
	// If set, the scheme uses the coarse name level.
	Coarse *SchemeLevel
	// Configuration for the fine name level.
	// If set, the fine name.
	Fine *SchemeLevel
	// Configuration for the case name level.
	// Always set.
	Case *SchemeLevel
}

// SchemeLevel represents a test hierarchy level in a test scheme.
type SchemeLevel struct {
	// The human readable name of the level.
	HumanReadableName string
	// The compiled validation regular expression.
	// May be nil if no validation is to be appled.
	ValidationRegexp *regexp.Regexp
}

// LegacyScheme is the built-in configuration for the special scheme "legacy".
// Treat as immutable.
var LegacyScheme = &Scheme{
	ID:                pbutil.LegacySchemeID,
	HumanReadableName: "Legacy test results",
	Case: &SchemeLevel{
		HumanReadableName: "Test Identifier",
	},
}

// Validate validates the given testID is for the given scheme
// and matches the criteria given by that scheme.
func (s Scheme) Validate(testID pbutil.BaseTestIdentifier) error {
	if testID.ModuleScheme != s.ID {
		return errors.Reason("module_scheme: expected test scheme %q but got scheme %q", s.ID, testID.ModuleScheme).Err()
	}
	if err := validateTestIDComponent(testID.CoarseName, testID.ModuleScheme, s.Coarse); err != nil {
		return errors.Annotate(err, "coarse_name").Err()
	}
	if err := validateTestIDComponent(testID.FineName, testID.ModuleScheme, s.Fine); err != nil {
		return errors.Annotate(err, "fine_name").Err()
	}
	if err := validateTestIDComponent(testID.CaseName, testID.ModuleScheme, s.Case); err != nil {
		return errors.Annotate(err, "case_name").Err()
	}
	return nil
}

func validateTestIDComponent(component, scheme string, level *SchemeLevel) error {
	if level != nil {
		if component == "" {
			return errors.Reason("required, please set a %s (scheme %q)", level.HumanReadableName, scheme).Err()
		}
		if level.ValidationRegexp != nil && !level.ValidationRegexp.MatchString(component) {
			return errors.Reason("does not match validation regexp %q, please set a valid %s (scheme %q)", level.ValidationRegexp.String(), level.HumanReadableName, scheme).Err()
		}
	} else {
		if component != "" {
			return errors.Reason("expected empty value (level is not defined by scheme %q)", scheme).Err()
		}
	}
	return nil
}

// FromProto constructs a new scheme from a resultdb response.
// This is used from ResultSink.
func FromProto(scheme *pb.Scheme) (*Scheme, error) {
	compiledScheme := &Scheme{
		ID:                scheme.Id,
		HumanReadableName: scheme.HumanReadableName,
	}
	if scheme.Coarse != nil {
		compiledLevel, err := newSchemeLevel(scheme.Coarse)
		if err != nil {
			return nil, err
		}
		compiledScheme.Coarse = compiledLevel
	}
	if scheme.Fine != nil {
		compiledLevel, err := newSchemeLevel(scheme.Fine)
		if err != nil {
			return nil, err
		}
		compiledScheme.Fine = compiledLevel
	}
	compiledLevel, err := newSchemeLevel(scheme.Case)
	if err != nil {
		return nil, err
	}
	compiledScheme.Case = compiledLevel
	return compiledScheme, nil
}

func newSchemeLevel(level *pb.Scheme_Level) (*SchemeLevel, error) {
	result := &SchemeLevel{
		HumanReadableName: level.HumanReadableName,
	}
	if level.ValidationRegexp != "" {
		compiledRegexp, err := regexp.Compile(level.ValidationRegexp)
		if err != nil {
			// This should never happen as the configuration has been validated.
			return nil, errors.Annotate(err, "could not compile validation regexp").Err()
		}
		result.ValidationRegexp = compiledRegexp
	}
	return result, nil
}

func (s *Scheme) ToProto() *pb.Scheme {
	return &pb.Scheme{
		Name:              formatSchemeName(s.ID),
		Id:                s.ID,
		HumanReadableName: s.HumanReadableName,
		Coarse:            schemeLevelProto(s.Coarse),
		Fine:              schemeLevelProto(s.Fine),
		Case:              schemeLevelProto(s.Case),
	}
}

func schemeLevelProto(level *SchemeLevel) *pb.Scheme_Level {
	if level == nil {
		return nil
	}
	regexp := ""
	if level.ValidationRegexp != nil {
		regexp = level.ValidationRegexp.String()
	}
	return &pb.Scheme_Level{
		HumanReadableName: level.HumanReadableName,
		ValidationRegexp:  regexp,
	}
}

func formatSchemeName(schemeID string) string {
	// No escaping required as schemeID uses a safe alphabet.
	return "schema/schemes/" + schemeID
}
