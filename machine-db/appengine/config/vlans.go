// Copyright 2017 The LUCI Authors.
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
	"net"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/config/validation"

	configPB "go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/model"
)

// vlansFilename is the name of the config file enumerating VLANs.
const vlansFilename = "vlans.cfg"

// vlanMaxId is the highest ID a VLAN may have.
const vlanMaxId = 65535

// vlanMinCIDRBlockSuffix is the minimum suffix of a CIDR block.
// This limits the length of CIDR blocks. Each block contains 2^(32 - suffix) IP addresses.
const vlanMinCIDRBlockSuffix = 18

// importVLANs fetches, validates, and applies VLAN configs.
func importVLANs(c context.Context, configSet config.Set) error {
	vlan := &configPB.VLANs{}
	metadata := &config.Meta{}
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, vlansFilename, textproto.Message(vlan), metadata); err != nil {
		return errors.Annotate(err, "failed to load %s", vlansFilename).Err()
	}
	logging.Infof(c, "Found %s revision %q", vlansFilename, metadata.Revision)

	ctx := &validation.Context{Context: c}
	ctx.SetFile(vlansFilename)
	validateVLANs(ctx, vlan)
	if err := ctx.Finalize(); err != nil {
		return errors.Annotate(err, "invalid config").Err()
	}

	if err := model.EnsureVLANs(c, vlan.Vlan); err != nil {
		return errors.Annotate(err, "failed to ensure VLANs").Err()
	}
	if err := model.EnsureIPs(c, vlan.Vlan); err != nil {
		return errors.Annotate(err, "failed to ensure IP addresses").Err()
	}
	return nil
}

// validateVLANs validates vlans.cfg.
func validateVLANs(c *validation.Context, cfg *configPB.VLANs) {
	// VLAN IDs must be unique.
	// Keep records of ones we've already seen.
	vlans := make(map[int64]struct{}, len(cfg.Vlan))
	for _, vlan := range cfg.Vlan {
		switch _, ok := vlans[vlan.Id]; {
		case vlan.Id < 1:
			c.Errorf("VLAN ID %d must be positive", vlan.Id)
		case vlan.Id > vlanMaxId:
			c.Errorf("VLAN ID %d must not exceed %d", vlan.Id, vlanMaxId)
		case ok:
			c.Errorf("duplicate VLAN %d", vlan.Id)
		}
		vlans[vlan.Id] = struct{}{}
		c.Enter("VLAN %d", vlan.Id)
		_, subnet, err := net.ParseCIDR(vlan.CidrBlock)
		if err != nil {
			c.Errorf("invalid CIDR block")
		} else {
			ones, _ := subnet.Mask.Size()
			if ones < vlanMinCIDRBlockSuffix {
				c.Errorf("CIDR block suffix must be at least %d", vlanMinCIDRBlockSuffix)
			}
		}
		c.Exit()
		// TODO(smut): Check that CIDR blocks are disjoint.
	}
}
