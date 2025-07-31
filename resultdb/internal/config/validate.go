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
	"strconv"
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
)

var gcsBucketRE = regexp.MustCompile(`^[a-z0-9_\.\-]{3,222}$`)
var schemeIDRE = regexp.MustCompile(`^` + SchemeIDPattern + `$`)
var humanReadableRE = regexp.MustCompile(`^[[:print:]]{1,100}$`)

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

// Validates according to https://cloud.google.com/storage/docs/objects#naming
func validateGCSBucketPrefix(ctx *validation.Context, name string, prefix string) {
	ctx.Enter("%s", name)
	defer ctx.Exit()

	prefixLen := len(prefix)
	if prefixLen < 1 || prefixLen > 1024 {
		ctx.Errorf("prefix: %q should have length between 1 and 1024 bytes", prefix)
	}
	if strings.HasPrefix(prefix, ".well-known/acme-challenge/") {
		ctx.Errorf("prefix: %q is not allowed", prefix)
	}
	if prefix == "." || prefix == ".." {
		ctx.Errorf("prefix: %q is not allowed, use '*' as wildcard to allow full access", prefix)
	}
	notAllowedChars, _ := strconv.Unquote(`"\u000a\u000b\u000c\u000d\u0085\u2028\u2029"`)
	if strings.ContainsAny(prefix, notAllowedChars) {
		ctx.Errorf("prefix: %q contains carriage return or line feed characters, which is not allowed", prefix)
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

	if proto.Size(&configpb.Config{Schemes: schemes}) > 100_000 {
		ctx.Errorf("too large; total size of configured schemes must not exceed 100 KB")
	}
	if len(schemes) > 1000 {
		ctx.Errorf("too large; may not exceed 1000 configured schemes")
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
