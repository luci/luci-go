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
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"

	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
)

// settingsID is the key for the service config entity in datastore.
const settingsID = "service_config"

// Settings represents the luci-notify configuration for a single instance of the service.
type Settings struct {
	// ID is the datastore ID for service settings.
	//
	// This should only ever be settingsID.
	ID string `gae:"$id"`

	// Revision is the revision of this instance's configuration.
	Revision string

	// MiloHost is the hostname of the Milo instance this luci-notify
	// instance should contact for additional build information.
	MiloHost string
}

// NewSettings constructs a new Settings from a revision and an unmarshaled proto config.
func NewSettings(revision string, settings *notifyConfig.Settings) *Settings {
	return &Settings{
		ID:       settingsID,
		Revision: revision,
		MiloHost: settings.MiloHost,
	}
}

// updateSettings fetches the service config from luci-config and then stores
// the new config into the datastore.
func updateSettings(c context.Context) error {
	// Load the settings from luci-config.
	cs := string(cfgclient.CurrentServiceConfigSet(c))
	lucicfg := backend.Get(c).GetConfigInterface(c, backend.AsService)
	cfg, err := lucicfg.GetConfig(c, cs, "settings.cfg", false)
	if err != nil {
		err = errors.Annotate(err, "loading settings.cfg from luci-config").Err()
		return err
	}

	settings, err := extractSettings(cfg)
	if err != nil {
		return err
	}
	newSettings := NewSettings(cfg.Revision, settings)

	// Do the revision check & swap in a datastore transaction.
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		oldSettings := Settings{ID: settingsID}
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
		if oldSettings.Revision == newSettings.Revision {
			logging.Debugf(c, "revisions matched (%s), no need to update", oldSettings.Revision)
			return nil
		}
		return datastore.Put(c, newSettings)
	}, nil)

	if err != nil {
		return err
	}
	return nil
}
