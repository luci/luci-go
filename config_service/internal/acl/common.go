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

package acl

import (
	"context"
	"fmt"
	"time"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/config_service/internal/common"
)

var (
	// ReadPermission allows caller to read the config set.
	ReadPermission = realms.RegisterPermission("configs.configSets.read")
	// ReadPermission allows caller to validate the config set.
	ValidatePermission = realms.RegisterPermission("configs.configSets.validate")
	// ReadPermission allows caller to re-import the config set.
	ReimportPermission = realms.RegisterPermission("configs.configSets.reimport")

	// aclCfgCache holds cache for acl.cfg content.
	aclCfgCache = caching.RegisterCacheSlot()
	// aclCfgCacheExpiration is the expiration time of aclCfgCache entry.
	aclCfgCacheExpiration = 10 * time.Minute
)

func getACLCfgCached(ctx context.Context) (*cfgcommonpb.AclCfg, error) {
	item, err := aclCfgCache.Fetch(ctx, func(any) (any, time.Duration, error) {
		aclCfg := &cfgcommonpb.AclCfg{}
		if err := common.LoadSelfConfig(ctx, common.ACLRegistryFilePath, aclCfg); err != nil {
			return nil, 0, err
		}
		return aclCfg, aclCfgCacheExpiration, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load ACL config: %w", err)
	}
	return item.(*cfgcommonpb.AclCfg), nil
}
