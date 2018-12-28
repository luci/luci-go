// Copyright 2018 The LUCI Authors.
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

// package config implements validation and common manipulation of CQ config
// files.
package config

import (
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"

	v1 "go.chromium.org/luci/cq/api/config/v1"
	v2 "go.chromium.org/luci/cq/api/config/v2"
)

// Config validation rules go here.

func init() {
	validation.Rules.Add("regex:projects/[^/]+", "cq.cfg", validateProjectCfg)
	validation.Rules.Add("regex:projects/[^/]+/refs/.+", "cq.cfg", validateRefCfg)
}

// validateRefCfg validates legacy ref-specific cq.cfg.
// Validation result is returned via validation ctx,
// while error returned directly implies only a bug in this code.
func validateRefCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := v1.Config{}
	if err := proto.UnmarshalText(string(content), &cfg); err != nil {
		ctx.Error(err)
		return nil
	}
	validateV1(ctx, &cfg)
	return nil
}

// validateProjectCfg validates project-level cq.cfg.
// Validation result is returned via validation ctx,
// while error returned directly implies only a bug in this code.
func validateProjectCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := v2.Config{}
	if err := proto.UnmarshalText(string(content), &cfg); err != nil {
		ctx.Error(err)
		return nil
	}
	// TODO(tandrii): implement.
	return errors.New("not implemented")
}
