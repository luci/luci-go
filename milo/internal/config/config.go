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

package config

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	protoutil "go.chromium.org/luci/common/proto"
	configInterface "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	configpb "go.chromium.org/luci/milo/proto/config"
)

const (
	// serviceConfigID is the key for the service config entity in datastore.
	serviceConfigID = "service_config"

	// globalConfigFilename is the name for milo's configuration file on
	// luci-config.
	globalConfigFilename = "settings.cfg"
)

// ServiceConfig is a container for the instance's service config.
type ServiceConfig struct {
	// ID is the datastore key.  This should be static, as there should only be
	// one service config.
	ID string `gae:"$id"`
	// Revision is the revision of the config, taken from luci-config.  This is used
	// to determine if the entry needs to be refreshed.
	Revision string
	// Data is the binary proto of the config.
	Data []byte `gae:",noindex"`
	// Text is the text format of the config.  For human consumption only.
	Text string `gae:",noindex"`
	// LastUpdated is the time this config was last updated.
	LastUpdated time.Time
}

// GetSettings returns the service (aka global) config for the current
// instance of Milo from the datastore.  Returns an empty config and warn heavily
// if none is found.
// TODO(hinoka): Use process cache to cache configs.
func GetSettings(c context.Context) *configpb.Settings {
	settings := configpb.Settings{}

	msg, err := GetCurrentServiceConfig(c)
	if err != nil {
		// The service config does not exist, just return an empty config
		// and complain loudly in the logs.
		logging.WithError(err).Errorf(c,
			"Encountered error while loading service config, using empty config.")
		return &settings
	}

	err = proto.Unmarshal(msg.Data, &settings)
	if err != nil {
		// The service config is broken, just return an empty config
		// and complain loudly in the logs.
		logging.WithError(err).Errorf(c,
			"Encountered error while unmarshalling service config, using empty config.")
		// Zero out the message just in case something got written in.
		settings = configpb.Settings{}
	}

	return &settings
}

var serviceCfgCache = caching.RegisterCacheSlot()

// GetCurrentServiceConfig gets the service config for the instance from either
// process cache or datastore cache.
func GetCurrentServiceConfig(c context.Context) (*ServiceConfig, error) {
	// This maker function is used to do the actual fetch of the ServiceConfig
	// from datastore.  It is called if the ServiceConfig is not in proc cache.
	item, err := serviceCfgCache.Fetch(c, func(any) (any, time.Duration, error) {
		msg := ServiceConfig{ID: serviceConfigID}
		err := datastore.Get(c, &msg)
		if err != nil {
			return nil, time.Minute, err
		}
		logging.Infof(c, "loaded service config from datastore")
		return msg, time.Minute, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get service config: %s", err.Error())
	}
	if msg, ok := item.(ServiceConfig); ok {
		logging.Infof(c, "loaded config entry from %s", msg.LastUpdated.Format(time.RFC3339))
		return &msg, nil
	}
	return nil, fmt.Errorf("could not load service config %#v", item)
}

// UpdateServiceConfig fetches the service config from luci-config
// and then stores a snapshot of the configuration in datastore.
func UpdateServiceConfig(c context.Context) (*configpb.Settings, error) {
	// Acquire the raw config client.
	content := ""
	meta := configInterface.Meta{}
	err := cfgclient.Get(c,
		"services/${appid}",
		globalConfigFilename,
		cfgclient.String(&content),
		&meta,
	)
	if err != nil {
		return nil, fmt.Errorf("could not load %s from luci-config: %s", globalConfigFilename, err)
	}

	// Reserialize it into a binary proto to make sure older/newer Milo versions
	// can safely use the entity when some fields are added/deleted. Text protos
	// do not guarantee that.
	settings := &configpb.Settings{}
	err = protoutil.UnmarshalTextML(content, settings)
	if err != nil {
		return nil, fmt.Errorf(
			"could not unmarshal proto from luci-config:\n%s", content)
	}
	newConfig := ServiceConfig{
		ID:          serviceConfigID,
		Text:        content,
		Revision:    meta.Revision,
		LastUpdated: time.Now().UTC(),
	}
	newConfig.Data, err = proto.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("could not marshal proto into binary\n%s", newConfig.Text)
	}

	// Do the revision check & swap in a datastore transaction.
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		oldConfig := ServiceConfig{ID: serviceConfigID}
		err := datastore.Get(c, &oldConfig)
		switch err {
		case datastore.ErrNoSuchEntity:
			// Might be the first time this has run.
			logging.WithError(err).Warningf(c, "No existing service config.")
		case nil:
			// Continue
		default:
			return fmt.Errorf("could not load existing config: %s", err)
		}
		// Check to see if we need to update
		if oldConfig.Revision == newConfig.Revision {
			logging.Infof(c, "revisions matched (%s), no need to update", oldConfig.Revision)
			return nil
		}
		logging.Infof(c, "revisions differ (old %s, new %s), updating",
			oldConfig.Revision, newConfig.Revision)
		return datastore.Put(c, &newConfig)
	}, nil)

	if err != nil {
		return nil, errors.Fmt("failed to update config entry in transaction: %w", err)
	}
	logging.Infof(c, "successfully updated to new config")

	return settings, nil
}

// UpdateConfigHandler is an HTTP handler that handles configuration update
// requests.
func UpdateConfigHandler(c context.Context) error {
	_, err := UpdateServiceConfig(c)
	if err != nil {
		return errors.Fmt("service update handler encountered error: %w", err)
	}

	return nil
}
