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

package config

import (
	"context"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/validation"

	gce "go.chromium.org/luci/gce/api/config/v1"
)

// cfgKey is the key to a *config.Interface in the context.
var cfgKey = "cfg"

// kindsFile is the name of the kinds config file.
const kindsFile = "kinds.cfg"

// vmsFile is the name of the VMs config file.
const vmsFile = "vms.cfg"

// WithInterface returns a new context with the given config.Interface installed.
func WithInterface(c context.Context, i config.Interface) context.Context {
	return context.WithValue(c, &cfgKey, i)
}

// getInterface returns the config.Interface installed in the current context.
func getInterface(c context.Context) config.Interface {
	return c.Value(&cfgKey).(config.Interface)
}

// fetch fetches configs from the config service.
func fetch(c context.Context) (*gce.Kinds, *gce.VMs, error) {
	cli := getInterface(c)
	set := cfgclient.CurrentServiceConfigSet(c)

	kinds := gce.Kinds{}
	cfg, err := cli.GetConfig(c, set, kindsFile, false)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to fetch %s", kindsFile).Err()
	}
	logging.Infof(c, "Found %s revision %q", kindsFile, cfg.Revision)
	rev := cfg.Revision
	if err := proto.UnmarshalText(cfg.Content, &kinds); err != nil {
		return nil, nil, errors.Annotate(err, "failed to load %s", kindsFile).Err()
	}

	vms := gce.VMs{}
	cfg, err = cli.GetConfig(c, set, vmsFile, false)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to fetch %s", vmsFile).Err()
	}
	logging.Infof(c, "Found %s revision %q", vmsFile, cfg.Revision)
	if cfg.Revision != rev {
		// Could happen if configs are being updated.
		return nil, nil, errors.Reason("config revision mismatch").Tag(transient.Tag).Err()
	}
	if err := proto.UnmarshalText(cfg.Content, &vms); err != nil {
		return nil, nil, errors.Annotate(err, "failed to load %s", vmsFile).Err()
	}
	return &kinds, &vms, nil
}

// validate validates configs.
func validate(c context.Context, kinds *gce.Kinds, vms *gce.VMs) error {
	v := &validation.Context{Context: c}
	v.SetFile(kindsFile)
	kinds.Validate(v)
	v.SetFile(vmsFile)
	vms.Validate(v, stringset.NewFromSlice(kinds.Names()...))
	return v.Finalize()
}

// merge merges validated configs.
// Each VMs block's referenced Kind is used to fill out unset values in the VMs block.
func merge(c context.Context, kinds *gce.Kinds, vms *gce.VMs) {
	kindsMap := kinds.Map()
	for _, block := range vms.Vms {
		if block.GetKind() != "" {
			// Merge the block's attributes into a copy of the kind's.
			// This ensures the block's attributes overwrite the kind's.
			attrs := proto.Clone(kindsMap[block.Kind].Attributes).(*gce.VM)
			// By default, proto.Merge concatenates repeated field values.
			// Instead, make repeated fields in the block override the kind.
			if len(block.Attributes.Disk) > 0 {
				attrs.Disk = nil
			}
			proto.Merge(attrs, block.Attributes)
			block.Attributes = attrs
		}
	}
}

// Import fetches and validates configs from the config service.
func Import(c context.Context) error {
	kinds, vms, err := fetch(c)
	if err != nil {
		return errors.Annotate(err, "failed to fetch configs").Err()
	}

	if err := validate(c, kinds, vms); err != nil {
		return errors.Annotate(err, "invalid configs").Err()
	}

	merge(c, kinds, vms)
	return nil
}
