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
	"google.golang.org/protobuf/encoding/prototext"

	configpb "go.chromium.org/luci/bisection/proto/config"
	"go.chromium.org/luci/config/validation"
)

const serviceConfigFilename = "config.cfg"

// init registers validation rules for config files
func init() {
	validation.Rules.Add("services/${appid}", serviceConfigFilename, validateServiceConfig)
}

// validateServiceConfig implements validation.Func and validates the content of
// the service-wide configuration file.
//
// Validation errors are returned via validation.Context
func validateServiceConfig(ctx *validation.Context, configSet, path string, content []byte) error {
	cfg := configpb.Config{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Errorf("invalid Config proto message: %s", err)
		return nil
	}

	ctx.SetFile(path)
	validateConfig(ctx, &cfg)
	return nil
}
