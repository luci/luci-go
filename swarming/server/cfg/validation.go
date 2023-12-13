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

package cfg

import (
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

func init() {
	addProtoValidator("settings.cfg", validateSettingsCfg)
	addProtoValidator("pools.cfg", validatePoolsCfg)
	addProtoValidator("bots.cfg", validateBotsCfg)
}

// addProtoValidator registers a validator for text proto service configs.
func addProtoValidator[T any, TP interface {
	*T
	proto.Message
}](path string, cb func(ctx *validation.Context, t *T)) {
	validation.Rules.Add("services/${appid}", path,
		func(ctx *validation.Context, _, path string, content []byte) error {
			ctx.SetFile(path)
			var msg TP = new(T)
			if err := prototext.Unmarshal(content, msg); err != nil {
				ctx.Error(err)
			} else {
				cb(ctx, msg)
			}
			return nil
		},
	)
}

// validateCipdPackage checks CIPD package name and version.
//
// The package name is allowed to be a template e.g. have "${platform}" and
// other substitutions inside.
func validateCipdPackage(ctx *validation.Context, pkg, ver string) {
	expanded, err := template.DefaultExpander().Expand(pkg)
	if err != nil {
		ctx.Errorf("bad package name template %q: %s", pkg, err)
	} else if err := common.ValidatePackageName(expanded); err != nil {
		ctx.Errorf("%s", err) // the error has all details already
	}
	if err := common.ValidateInstanceVersion(ver); err != nil {
		ctx.Errorf("%s", err) // the error has all details already
	}
}

// validateDimensionKey checks if `key` can be a dimension key.
func validateDimensionKey(key string) error {
	if key == "" {
		return errors.Reason("the key cannot be empty").Err()
	}
	// TODO(vadimsh): Implement.
	return nil
}

// validateDimensionValue checks if `val` can be a dimensions value
func validateDimensionValue(val string) error {
	if val == "" {
		return errors.Reason("the value cannot be empty").Err()
	}
	// TODO(vadimsh): Implement.
	return nil
}
