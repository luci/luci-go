// Copyright 2022 The LUCI Authors.
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

package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
)

const (
	// The pattern of a valid test scheme, excluding ^ and $.
	SchemeIDPattern = `[a-z][a-z0-9]{0,19}`

	unspecifiedMessage = "unspecified"

	maxSchemesConfigSize = 100 * 1024 // bytes
	maxSchemes           = 1000       // schemes

	maxProducerSystemsConfigSize = 100 * 1024 // bytes
	maxProducerSystems           = 1000       // producer systems
)

var gcsBucketRE = regexp.MustCompile(`^[a-z0-9_\.\-]{3,222}$`)
var schemeIDRE = regexp.MustCompile(`^` + SchemeIDPattern + `$`)
var humanReadableRE = regexp.MustCompile(`^[[:print:]]{1,100}$`)

// templateVarRE matches a template variable, e.g. "${run_id}".
var templateVarRE = regexp.MustCompile(`\$\{(.*?)\}`)

func validateStringConfig(ctx *validation.Context, name, cfg string, re *regexp.Regexp) {
	ctx.Enter("%s", name)
	defer ctx.Exit()
	if cfg == "" {
		ctx.Errorf("empty %s is not allowed", name)
		return
	}
	if !re.MatchString(cfg) {
		ctx.Errorf("invalid %s: %q", name, cfg)
	}
}

func validateGCSAllowlist(ctx *validation.Context, name string, allowList *configpb.GcsAllowList) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	if len(allowList.Users) == 0 {
		ctx.Errorf("users must have at least one user")
	}
	for _, user := range allowList.Users {
		identity, err := identity.MakeIdentity(user)
		if err != nil {
			ctx.Errorf("%s", err.Error())
		}
		err = identity.Validate()
		if err != nil {
			ctx.Errorf("%s", err.Error())
		}
	}

	if len(allowList.Buckets) == 0 {
		ctx.Errorf("buckets must have at least one bucket")
	}
	for _, bucket := range allowList.Buckets {
		validateStringConfig(ctx, "bucket", bucket, gcsBucketRE)
	}
}

// validateProjectConfigRaw deserializes the project-level config message
// and passes it through the validator.
func validateProjectConfigRaw(ctx *validation.Context, content string) *configpb.ProjectConfig {
	msg := &configpb.ProjectConfig{}
	if err := luciproto.UnmarshalTextML(content, msg); err != nil {
		ctx.Errorf("failed to unmarshal as text proto: %s", err)
		return nil
	}
	validateProjectConfig(ctx, msg)
	return msg
}

func validateProjectConfig(ctx *validation.Context, cfg *configpb.ProjectConfig) {
	for i, allowList := range cfg.GcsAllowList {
		validateGCSAllowlist(ctx, fmt.Sprintf("gcs_allow_list[%d]", i), allowList)
	}
}

func validateServiceConfig(ctx *validation.Context, cfg *configpb.Config) {
	if cfg.BqArtifactExportConfig == nil {
		ctx.Errorf("missing BQ artifact export config")
		return
	}
	validateBQArtifactExportConfig(ctx, cfg.BqArtifactExportConfig)
	validateSchemes(ctx, cfg.Schemes)
	validateProducerSystems(ctx, cfg.ProducerSystems)
	validateAndroidBuild(ctx, cfg.AndroidBuild)
}

func validateBQArtifactExportConfig(ctx *validation.Context, cfg *configpb.BqArtifactExportConfig) {
	ctx.Enter("bq_artifact_export_config")
	defer ctx.Exit()
	if cfg.ExportPercent < 0 || cfg.ExportPercent > 100 {
		ctx.Errorf("export percent must be between 0 and 100")
	}
}

func validateSchemes(ctx *validation.Context, schemes []*configpb.Scheme) {
	ctx.Enter("schemes")
	defer ctx.Exit()

	if proto.Size(&configpb.Config{Schemes: schemes}) > maxSchemesConfigSize {
		ctx.Errorf("too large; total size of configured schemes must not exceed %d bytes", maxSchemesConfigSize)
	}
	if len(schemes) > maxSchemes {
		ctx.Errorf("too large; may not exceed %d configured schemes", maxSchemes)
	}

	seenIDs := map[string]struct{}{}
	for i, scheme := range schemes {
		validateScheme(ctx, fmt.Sprintf("[%v]", i), scheme, seenIDs)
	}
}

func validateScheme(ctx *validation.Context, name string, cfg *configpb.Scheme, seenIDs map[string]struct{}) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	validateSchemeID(ctx, cfg.Id, seenIDs)
	validateHumanReadableName(ctx, cfg.HumanReadableName)

	if cfg.Coarse != nil && cfg.Fine == nil {
		ctx.Errorf("invalid combination of levels, got coarse set and fine unset; if only one level is to be used, configure the fine level instead of the coarse level")
	}
	if cfg.Coarse != nil {
		validateSchemeLevel(ctx, "coarse", cfg.Coarse)
	}
	if cfg.Fine != nil {
		validateSchemeLevel(ctx, "fine", cfg.Fine)
	}
	validateSchemeLevel(ctx, "case", cfg.Case)
}

func validateSchemeID(ctx *validation.Context, id string, seenIDs map[string]struct{}) {
	ctx.Enter("id")
	defer ctx.Exit()

	if id == pbutil.LegacySchemeID {
		ctx.Errorf(`%q is a reserved built-in scheme and cannot be configured`, pbutil.LegacySchemeID)
	}
	if err := validate.SpecifiedWithRe(schemeIDRE, id); err != nil {
		ctx.Error(err)
	}
	if _, ok := seenIDs[id]; ok {
		ctx.Errorf("scheme with ID %q appears in collection more than once", id)
	}
	seenIDs[id] = struct{}{}
}

func validateHumanReadableName(ctx *validation.Context, value string) {
	ctx.Enter("human_readable_name")
	defer ctx.Exit()

	if err := validate.SpecifiedWithRe(humanReadableRE, value); err != nil {
		ctx.Error(err)
	}
}

func validateSchemeLevel(ctx *validation.Context, name string, level *configpb.Scheme_Level) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	if level == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	validateHumanReadableName(ctx, level.HumanReadableName)
	validateValidationRegexp(ctx, level.ValidationRegexp)
}

func validateValidationRegexp(ctx *validation.Context, pattern string) {
	ctx.Enter("validation_regexp")
	defer ctx.Exit()

	if pattern == "" {
		// Empty pattern means that no additional validation should be applied.
		return
	}
	if !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, "$") {
		ctx.Errorf("pattern must start and end with ^ and $")
		return
	}
	_, err := regexp.Compile(pattern)
	if err != nil {
		ctx.Errorf("could not compile pattern: %s", err)
	}
}

// validateProducerSystems validates the producer systems configuration.
func validateProducerSystems(ctx *validation.Context, systems []*configpb.ProducerSystem) {
	ctx.Enter("producer_systems")
	defer ctx.Exit()

	if proto.Size(&configpb.Config{ProducerSystems: systems}) > maxProducerSystemsConfigSize {
		ctx.Errorf("too large; total size of configured producer systems must not exceed %d bytes", maxProducerSystemsConfigSize)
	}
	if len(systems) > maxProducerSystems {
		ctx.Errorf("too large; may not exceed %d configured producer systems", maxProducerSystems)
	}

	seenSystems := map[string]struct{}{}
	for i, system := range systems {
		validateProducerSystem(ctx, fmt.Sprintf("[%v]", i), system, seenSystems)
	}
}

// validateProducerSystem validates the configuration for a producer system.
func validateProducerSystem(ctx *validation.Context, name string, cfg *configpb.ProducerSystem, seenSystems map[string]struct{}) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	validateProducerSystemName(ctx, cfg.System, seenSystems)
	validatePattern(ctx, "name_pattern", cfg.NamePattern)
	validatePattern(ctx, "data_realm_pattern", cfg.DataRealmPattern)

	// Validate URL templates.
	re, err := regexp.Compile(cfg.NamePattern)
	if err != nil {
		// We require the name pattern to be valid. If it is not,
		// an error will already have been reported.
		return
	}

	allowedVars := map[string]struct{}{"data_realm": {}}
	for _, name := range re.SubexpNames() {
		if name != "" {
			allowedVars[name] = struct{}{}
		}
	}
	validateURLTemplate(ctx, "url_template", cfg.UrlTemplate, allowedVars)
	for realm, template := range cfg.UrlTemplateByDataRealm {
		validateURLTemplate(ctx, fmt.Sprintf("url_template_by_data_realm[%q]", realm), template, allowedVars)
	}
}

// validateProducerSystemName validates the name of a producer system.
func validateProducerSystemName(ctx *validation.Context, system string, seenSystems map[string]struct{}) {
	ctx.Enter("system")
	defer ctx.Exit()

	if err := pbutil.ValidateProducerSystemName(system); err != nil {
		ctx.Error(err)
		return
	}
	if _, ok := seenSystems[system]; ok {
		ctx.Errorf("producer system with name %q appears in collection more than once", system)
	}
	seenSystems[system] = struct{}{}
}

// validatePattern validates the pattern is a valid, non-empty RE2 regular expression,
// and that it starts and ends with ^ and $.
func validatePattern(ctx *validation.Context, name string, pattern string) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	if pattern == "" {
		ctx.Errorf("unspecified")
		return
	}
	if !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, "$") {
		ctx.Errorf("pattern must start and end with ^ and $")
		return
	}
	if _, err := regexp.Compile(pattern); err != nil {
		ctx.Errorf("could not compile pattern: %s", err)
	}
}

// validateURLTemplate validates the URL template.
func validateURLTemplate(ctx *validation.Context, name, template string, allowedVars map[string]struct{}) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	if template == "" {
		return
	}
	// This validation is not perfect as it does not handle bad syntax (e.g. "${foo"), but it
	// does pick up some obvious errors like references to unknown variables.
	matches := templateVarRE.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		varName := match[1]
		if _, ok := allowedVars[varName]; !ok {
			ctx.Errorf("unknown variable %q; allowed variables are %s", varName, allowedVarsList(allowedVars))
			return
		}
	}
}

// allowedVarsList returns a string representation of the allowed variables.
func allowedVarsList(allowedVars map[string]struct{}) string {
	vars := make([]string, 0, len(allowedVars))
	for v := range allowedVars {
		vars = append(vars, fmt.Sprintf("%q", v))
	}
	sort.Strings(vars)
	return fmt.Sprintf("[%s]", strings.Join(vars, ", "))
}

func validateAndroidBuild(ctx *validation.Context, cfg *configpb.AndroidBuild) {
	if cfg == nil {
		return
	}
	ctx.Enter("android_build")
	defer ctx.Exit()

	validatePattern(ctx, "data_realm_pattern", cfg.DataRealmPattern)

	for realm, cfg := range cfg.DataRealms {
		validateByDataRealmConfig(ctx, fmt.Sprintf("data_realms[%q]", realm), cfg)
	}
}

func validateByDataRealmConfig(ctx *validation.Context, name string, cfg *configpb.AndroidBuild_ByDataRealmConfig) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	if cfg == nil {
		ctx.Errorf("unspecified")
		return
	}

	allowedVars := map[string]struct{}{
		"branch":       {},
		"build_target": {},
		"build_id":     {},
	}
	validateURLTemplate(ctx, "full_build_url_template", cfg.FullBuildUrlTemplate, allowedVars)
}
