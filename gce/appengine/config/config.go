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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/config/validation"

	gce "go.chromium.org/luci/gce/api/config/v1"
)

// kindsFile is the name of the kinds config file.
const kindsFile = "kinds.cfg"

// vmsFile is the name of the VMs config file.
const vmsFile = "vms.cfg"

// fetch fetches configs from the config service.
func fetch(c context.Context) (*gce.Kinds, *gce.VMs, error) {
	as := cfgclient.AsService
	meta := &config.Meta{}
	set := cfgclient.CurrentServiceConfigSet(c)

	kinds := &gce.Kinds{}
	if err := cfgclient.Get(c, as, set, kindsFile, textproto.Message(kinds), meta); err != nil {
		return nil, nil, errors.Annotate(err, "failed to load %s", kindsFile).Err()
	}
	logging.Infof(c, "Found %s revision %q", kindsFile, meta.Revision)
	rev := meta.Revision

	vms := &gce.VMs{}
	if err := cfgclient.Get(c, as, set, vmsFile, textproto.Message(vms), meta); err != nil {
		return nil, nil, errors.Annotate(err, "failed to load %s", vmsFile).Err()
	}
	logging.Infof(c, "Found %s revision %q", vmsFile, meta.Revision)
	if meta.Revision != rev {
		// Could happen if configs are being updated.
		return nil, nil, errors.Reason("config revision mismatch").Tag(transient.Tag).Err()
	}
	return kinds, vms, nil
}

// validate validates configs.
func validate(c context.Context, kinds *gce.Kinds, vms *gce.VMs) error {
	v := &validation.Context{Context: c}
	v.SetFile(kindsFile)
	kinds.Validate(v)
	v.SetFile(vmsFile)
	vms.Validate(v, stringset.NewFromSlice(kinds.Names()...))
	if err := v.Finalize(); err != nil {
		return errors.Annotate(err, "invalid configs").Err()
	}
	return nil
}

// Import fetches and validates configs from the config service.
func Import(c context.Context) error {
	kinds, vms, err := fetch(c)
	if err != nil {
		return errors.Annotate(err, "failed to fetch configs").Err()
	}

	if err := validate(c, kinds, vms); err != nil {
		return err
	}
	return nil
}
