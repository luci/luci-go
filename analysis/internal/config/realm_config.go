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
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

// RealmNotExistsErr is returned if no configuration could be found
// for the specified realm.
var RealmNotExistsErr = errors.New("no config found for realm")

// Realm returns the configurations of the requested realm.
// If no configuration can be found for the realm, returns
// RealmNotExistsErr.
func Realm(ctx context.Context, global string) (*configpb.RealmConfig, error) {
	project, realm := realms.Split(global)
	pc, err := Project(ctx, project)
	if err != nil {
		return nil, err
	}
	for _, rc := range pc.GetRealms() {
		if rc.Name == realm {
			return rc, nil
		}
	}
	return nil, RealmNotExistsErr
}
