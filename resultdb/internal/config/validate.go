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

	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth/realms"

	configpb "go.chromium.org/luci/resultdb/proto/config"
)

var GCSBucketRE = regexp.MustCompile(`^[a-z0-9_\.\-]{3,222}$`)

func validateStringConfig(ctx *validation.Context, name, cfg string, re *regexp.Regexp) {
	ctx.Enter(name)
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
	ctx.Enter(name)
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

func validateRealmGCSAllowlist(ctx *validation.Context, name string, allowList *configpb.RealmGcsAllowList) {
	ctx.Enter(name)
	defer ctx.Exit()

	err := realms.ValidateRealmName(allowList.Realm, realms.ProjectScope)
	if err != nil {
		ctx.Errorf(err.Error())
	}
	for _, prefixes := range allowList.GcsBucketPrefixes {
		validateStringConfig(ctx, "bucket", prefixes.Bucket, GCSBucketRE)
		for i, prefix := range prefixes.AllowedPrefixes {
			validateGCSBucketPrefix(ctx, fmt.Sprintf("allowed_prefixes[%d]", i), prefix)
		}
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
	realms := make(map[string]bool, len(cfg.RealmGcsAllowlist))
	for i, allowList := range cfg.RealmGcsAllowlist {
		realm := allowList.Realm
		if _, ok := realms[realm]; ok {
			ctx.Errorf("realm: %s is configured more than once", realm)
		}
		realms[realm] = true
		validateRealmGCSAllowlist(ctx, fmt.Sprintf("realm_gcs_allowlist[%d]", i), allowList)
	}
}
