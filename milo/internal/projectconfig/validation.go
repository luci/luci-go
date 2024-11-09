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

package projectconfig

import (
	"regexp"
	"strings"

	bbprotoutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	protoutil "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
)

// Config validation rules go here.

const maxLenPropertiesSchema = 256

var (
	propertiesSchemaRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)
	displayNameRe      = regexp.MustCompile(`^[[:print:]]{1,64}$`)
	displayItemPathRe  = regexp.MustCompile(`^([^.]+\.)*[^.]+$`)
)

func init() {
	// Milo is only responsible for validating the config matching the instance's
	// appID in a project config.
	validation.Rules.Add("regex:projects/.*", "${appid}.cfg", validateProjectCfg)
}

// validateProjectCfg implements validation.Func by taking a potential Milo
// config at path, validating it, and writing the result into ctx.
//
// The validation we do include:
//
// * Make sure the config is able to be unmarshalled.
// * Make sure all consoles have either builder_view_only: true or manifest_name
func validateProjectCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	proj := projectconfigpb.Project{}
	if err := protoutil.UnmarshalTextML(string(content), &proj); err != nil {
		ctx.Error(err)
		return nil
	}
	knownHeaders := stringset.New(len(proj.Headers))
	for i, header := range proj.Headers {
		ctx.Enter("header #%d (%s)", i, header.Id)
		if header.Id == "" {
			ctx.Errorf("missing id")
		} else if !knownHeaders.Add(header.Id) {
			ctx.Errorf("duplicate header id")
		}
		ctx.Exit()
	}

	knownConsoles := stringset.New(len(proj.Consoles))
	for i, console := range proj.Consoles {
		ctx.Enter("console #%d (%s)", i, console.Id)
		validateConsole(ctx, &knownConsoles, &knownHeaders, console)
		ctx.Exit()
	}
	if proj.LogoUrl != "" && !strings.HasPrefix(proj.LogoUrl, "https://storage.googleapis.com/") {
		ctx.Errorf("invalid logo url %q, must begin with https://storage.googleapis.com/", proj.LogoUrl)
	}

	for i, builderID := range proj.IgnoredBuilderIds {
		ctx.Enter("ignored builder ID #%d (%s)", i, builderID)
		if strings.Count(builderID, "/") != 1 {
			ctx.Errorf("invaid builder ID, the format must be <bucket>/<builder>")
		}
		ctx.Exit()
	}
	if proj.MetadataConfig != nil {
		validateMetadataConfig(ctx, proj.MetadataConfig)
	}

	return nil
}

func validateConsole(ctx *validation.Context, knownConsoles *stringset.Set, knownHeaders *stringset.Set, console *projectconfigpb.Console) {
	if console.Id == "" {
		ctx.Errorf("missing id")
	} else if strings.ContainsAny(console.Id, "/") {
		// unfortunately httprouter uses decoded path when performing URL routing
		// therefore we can't use '/' in the console ID. Other chars are safe as long as we encode them
		ctx.Errorf("id can not contain '/'")
	} else if !knownConsoles.Add(console.Id) {
		ctx.Errorf("duplicate console")
	}
	isExternalConsole := console.ExternalProject != "" || console.ExternalId != ""
	if isExternalConsole {
		validateExternalConsole(ctx, console)
	} else {
		validateLocalConsole(ctx, knownHeaders, console)
	}
}

func validateLocalConsole(ctx *validation.Context, knownHeaders *stringset.Set, console *projectconfigpb.Console) {
	// If this is a CI console and it's missing manifest name, the author
	// probably forgot something.
	if !console.BuilderViewOnly {
		if console.ManifestName == "" {
			ctx.Errorf("ci console missing manifest name")
		}
		if console.RepoUrl == "" {
			ctx.Errorf("ci console missing repo url")
		}
		if len(console.Refs) == 0 {
			ctx.Errorf("ci console missing refs")
		} else {
			gitiles.ValidateRefSet(ctx, console.Refs)
		}
	} else {
		if console.IncludeExperimentalBuilds {
			ctx.Errorf("builder_view_only and include_experimental_builds both set")
		}
	}

	if console.HeaderId != "" && !knownHeaders.Has(console.HeaderId) {
		ctx.Errorf("header %s not defined", console.HeaderId)
	}
	if console.HeaderId != "" && console.Header != nil {
		ctx.Errorf("cannot specify both header and header_id")
	}
	for j, b := range console.Builders {
		ctx.Enter("builders #%d", j+1)
		if b.Name == "" && b.Id == nil {
			ctx.Errorf("name must be non-empty or id must be set")
		}

		// `id` take precedence over `name`. If `id` is specified `name` is simply
		// discarded.
		if b.Id != nil {
			if err := bbprotoutil.ValidateRequiredBuilderID(b.Id); err != nil {
				ctx.Errorf(`id: %v`, err)
			}
		} else if b.Name != "" {
			_, err := utils.ParseLegacyBuilderID(b.Name)
			if err != nil {
				ctx.Errorf(`name: %v`, err)
			}
		}
		ctx.Exit()
	}
}

func validateExternalConsole(ctx *validation.Context, console *projectconfigpb.Console) {
	// Verify that both project and external ID are set.
	if console.ExternalProject == "" {
		ctx.Errorf("missing external project")
	}
	if console.ExternalId == "" {
		ctx.Errorf("missing external console id")
	}

	// Verify that external consoles have no local-console-only fields.
	if console.RepoUrl != "" {
		ctx.Errorf("repo url found in external console")
	}
	if len(console.Refs) > 0 {
		ctx.Errorf("refs found in external console")
	}
	if console.ManifestName != "" {
		ctx.Errorf("manifest name found in external console")
	}
	if len(console.Builders) > 0 {
		ctx.Errorf("builders found in external console")
	}
	if console.HeaderId != "" || console.Header != nil {
		ctx.Errorf("header found in external console")
	}
}

func validateMetadataConfig(ctx *validation.Context, mc *projectconfigpb.MetadataConfig) {
	ctx.Enter("metadata config")
	defer ctx.Exit()
	for i, testMetadataProperties := range mc.TestMetadataProperties {
		ctx.Enter("test metadata properties #%d", i)
		validateTestMetadataProperty(ctx, testMetadataProperties)
		ctx.Exit()
	}
}

func validateTestMetadataProperty(ctx *validation.Context, dr *projectconfigpb.DisplayRule) {
	if len(dr.Schema) > maxLenPropertiesSchema {
		ctx.Errorf("schema exceeds the maximum size of %d bytes", maxLenPropertiesSchema)
	}
	validateWithRe(ctx, propertiesSchemaRe, dr.Schema, "schema")
	for i, displayItem := range dr.DisplayItems {
		ctx.Enter("display item #%d", i)
		validateWithRe(ctx, displayNameRe, displayItem.DisplayName, "displayName")
		validateWithRe(ctx, displayItemPathRe, displayItem.Path, "path")
		ctx.Exit()
	}
}

func validateWithRe(ctx *validation.Context, re *regexp.Regexp, value string, name string) {
	ctx.Enter("%s", name)
	if value == "" {
		ctx.Errorf("unspecified")
		return
	}
	if !re.MatchString(value) {
		ctx.Errorf("does not match %s", re)
	}
	ctx.Exit()
}
