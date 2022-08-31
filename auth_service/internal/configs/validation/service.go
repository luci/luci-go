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

package validation

import (
	"regexp"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"
)

// ipAllowlistNameRE is the regular expression for IP Allowlist Names.
var ipAllowlistNameRE = regexp.MustCompile(`^[0-9A-Za-z_\-\+\.\ ]{2,200}$`)

// validateAllowlist validates an ip_allowlist.cfg file.
//
// Validation result is returned via validation ctx, while error returned
// drectly implies only a bug in this code.
func validateAllowlist(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := configspb.IPAllowlistConfig{}

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}

	allowlists := stringset.New(len(cfg.GetIpAllowlists()))

	for _, a := range cfg.GetIpAllowlists() {
		switch name := a.GetName(); {
		case !ipAllowlistNameRE.MatchString(name):
			ctx.Errorf("invalid ip allowlist name %s", name)
		case allowlists.Has(name):
			ctx.Errorf("ip allowlist is defined twice %s", name)
		default:
			allowlists.Add(name)
		}
		// TODO(cjacomet): Validate subnets.
	}
	return nil
}