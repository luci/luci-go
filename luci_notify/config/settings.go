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
	"fmt"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

// Settings represents the luci-notify configuration for a single instance of the service.
type Settings struct {
	// ID is the datastore ID for service settings.
	ID string `gae:"$id,service_config"`

	// Revision is the revision of this instance's configuration.
	Revision string

	// Settings is an embedded copy of this instance's configuration proto.
	Settings notifypb.Settings `gae:"-"`
}

// Load loads a Settings's information from props.
//
// This implements PropertyLoadSaver. Load unmarshals the property Settings
// stored in the datastore as a binary proto into the struct's Settings field.
func (s *Settings) Load(props datastore.PropertyMap) error {
	if pdata, ok := props["Settings"]; ok {
		settings := pdata.Slice()
		if len(settings) != 1 {
			return fmt.Errorf("property `Settings` is a property slice")
		}
		settingsBytes, ok := settings[0].Value().([]byte)
		if !ok {
			return fmt.Errorf("expected byte array for property `Settings`")
		}
		if err := proto.Unmarshal(settingsBytes, &s.Settings); err != nil {
			return err
		}
		delete(props, "Settings")
	}
	return datastore.GetPLS(s).Load(props)
}

// Save saves a Settings's information to a property map.
//
// This implements PropertyLoadSaver. Save marshals the Settings
// field as a binary proto and stores it in the Settings property.
func (s *Settings) Save(withMeta bool) (datastore.PropertyMap, error) {
	props, err := datastore.GetPLS(s).Save(withMeta)
	if err != nil {
		return nil, err
	}
	settingsBytes, err := proto.Marshal(&s.Settings)
	if err != nil {
		return nil, err
	}
	props["Settings"] = datastore.MkProperty(settingsBytes)
	return props, nil
}

// updateSettings fetches the service config from luci-config and then stores
// the new config into the datastore.
func updateSettings(c context.Context) error {
	// Load the settings from luci-config.
	lucicfg := cfgclient.Client(c)
	cfg, err := lucicfg.GetConfig(c, "services/${appid}", "settings.cfg", false)
	if err != nil {
		return errors.Annotate(err, "loading settings.cfg from luci-config").Err()
	}

	// Do the revision check & swap in a datastore transaction.
	return datastore.RunInTransaction(c, func(c context.Context) error {
		oldSettings := Settings{}
		err := datastore.Get(c, &oldSettings)
		switch err {
		case datastore.ErrNoSuchEntity:
			// This might be the first time this has run, so a warning here is ok.
			logging.WithError(err).Warningf(c, "no existing service config")
		case nil:
			// Continue
		default:
			return errors.Annotate(err, "loading existing config").Err()
		}
		// Check to see if we need to update
		if oldSettings.Revision == cfg.Revision {
			logging.Debugf(c, "revisions matched (%s), no need to update", cfg.Revision)
			return nil
		}
		newSettings := Settings{Revision: cfg.Revision}
		if err := proto.UnmarshalText(cfg.Content, &newSettings.Settings); err != nil {
			return errors.Annotate(err, "unmarshalling proto").Err()
		}
		ctx := &validation.Context{Context: c}
		ctx.SetFile("settings.cfg")
		validateSettings(ctx, &newSettings.Settings)
		if err := ctx.Finalize(); err != nil {
			return errors.Annotate(err, "validating settings").Err()
		}
		return datastore.Put(c, &newSettings)
	}, nil)
}

func FetchSettings(ctx context.Context) (*notifypb.Settings, error) {
	settings := Settings{}
	err := datastore.Get(ctx, &settings)
	if err != nil {
		return nil, errors.Annotate(err, "loading existing config").Err()
	}
	return &settings.Settings, nil
}
