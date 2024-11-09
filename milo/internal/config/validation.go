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

package config

import (
	protoutil "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/milo/internal/git/gitacls"
	configpb "go.chromium.org/luci/milo/proto/config"
)

// Config validation rules go here.
func init() {
	validation.Rules.Add("services/${appid}", globalConfigFilename, validateServiceCfg)
}

// validateServiceCfg implements validation.Func by taking a potential Milo
// service global config, validating it, and writing the result into ctx.
//
// The validation we do include:
//
// * Make sure the config is able to be unmarshalled.
func validateServiceCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	settings := configpb.Settings{}
	if err := protoutil.UnmarshalTextML(string(content), &settings); err != nil {
		ctx.Error(err)
	}
	gitacls.ValidateConfig(ctx, settings.SourceAcls)
	return nil
}
