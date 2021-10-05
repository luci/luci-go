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

// Package validation implements validation and common manipulation of CQ config
// files.
package validation

import (
	"go.chromium.org/luci/config/validation"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

// ValidateProject validates project config and returns error only on blocking
// errors (ie ignores problems with warning severity).
func ValidateProject(cfg *cfgpb.Config) error {
	ctx := validation.Context{}
	validateProjectConfig(&ctx, cfg)
	switch verr, ok := ctx.Finalize().(*validation.Error); {
	case !ok:
		return nil
	default:
		return verr.WithSeverity(validation.Blocking)
	}
}
