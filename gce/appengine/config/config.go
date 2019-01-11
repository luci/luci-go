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
func fetch(c context.Context) (*gce.Kinds, *gce.Configs, error) {
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

	cfgs := gce.Configs{}
	cfg, err = cli.GetConfig(c, set, vmsFile, false)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to fetch %s", vmsFile).Err()
	}
	logging.Infof(c, "Found %s revision %q", vmsFile, cfg.Revision)
	if cfg.Revision != rev {
		// Could happen if configs are being updated.
		return nil, nil, errors.Reason("config revision mismatch").Tag(transient.Tag).Err()
	}
	if err := proto.UnmarshalText(cfg.Content, &cfgs); err != nil {
		return nil, nil, errors.Annotate(err, "failed to load %s", vmsFile).Err()
	}
	return &kinds, &cfgs, nil
}

// validate validates configs.
func validate(c context.Context, kinds *gce.Kinds, cfgs *gce.Configs) error {
	v := &validation.Context{Context: c}
	v.SetFile(kindsFile)
	kinds.Validate(v)
	v.SetFile(vmsFile)
	cfgs.Validate(v, stringset.NewFromSlice(kinds.Names()...))
	return v.Finalize()
}

// merge merges validated configs.
// Each config's referenced Kind is used to fill out unset values in its attributes.
func merge(c context.Context, kinds *gce.Kinds, cfgs *gce.Configs) {
	kindsMap := kinds.Map()
	for _, cfg := range cfgs.Vms {
		if cfg.GetKind() != "" {
			// Merge the config's attributes into a copy of the kind's.
			// This ensures the config's attributes overwrite the kind's.
			attrs := proto.Clone(kindsMap[cfg.Kind].Attributes).(*gce.VM)
			// By default, proto.Merge concatenates repeated field values.
			// Instead, make repeated fields in the config override the kind.
			if len(cfg.Attributes.Disk) > 0 {
				attrs.Disk = nil
			}
			proto.Merge(attrs, cfg.Attributes)
			cfg.Attributes = attrs
		}
	}
}

// Import fetches and validates configs from the config service.
func Import(c context.Context) error {
	kinds, cfgs, err := fetch(c)
	if err != nil {
		return errors.Annotate(err, "failed to fetch configs").Err()
	}

	if err := validate(c, kinds, cfgs); err != nil {
		return errors.Annotate(err, "invalid configs").Err()
	}

	merge(c, kinds, cfgs)
	return nil
}
