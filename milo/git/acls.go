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

package git

import (
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/milo/api/config"
)

func AclsFromConfig(cfg *config.Settings_SourceAcls) (*Acls, error) {
	var ctx validation.Context
	acls, err := validateAndLoadAcls(ctx, ctx){
	}
}

func ValidateAclsConfig(ctx *validation.Context, cfg *config.Settings_SourceAcls) error {
	_, err := validateAndLoadAcls(cfg, ctx)
	return err
}

// Acls defines who can read git repositories and corresponding Gerrit CLs.
type Acls struct {
	hosts map[string]hostAcls
}

// private implementation.

func validateAndLoadAcls(ctx *validation.Context, cfg *config.Settings_SourceAcls) (*Acls, error) {
}

type hostAcls struct {
	readers []string
	byRepo  map[string][]string
}
