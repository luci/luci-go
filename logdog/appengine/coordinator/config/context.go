// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package config

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// UseConfig sets up luci-config as a remote configuration source based on the
// current GlobalConfig.
//
// If the GlobalConfig doesn't fully specify a luci-config source, no
// luci-config will be installed.
func UseConfig(c *context.Context) error {
	if info.Get(*c).IsDevAppServer() {
		log.Infof(*c, "Development server detected; installing development config.")

		mcfg, err := devAppServerMemoryConfig(*c)
		if err != nil {
			return err
		}
		*c = config.Set(*c, memory.New(mcfg))
		return nil
	}

	// Install our GAE configuration.
	ci, err := gaeconfig.New(*c)
	if err != nil {
		log.WithError(err).Errorf(*c, "Failed to install config service.")
		return err
	}

	*c = config.Set(*c, ci)
	return nil
}

// DevConfig is an auxiliary development configuration. It is only active when
// running under the dev appserver.
type devConfig struct {
	_kind string `gae:"$kind,DevConfig"`
	_id   int64  `gae:"$id,1"`

	// Config, is set, is the application's text-format configuration protobuf.
	// This is only used when running under dev-appserver.
	Config string `gae:",noindex"`
}

func devAppServerMemoryConfig(c context.Context) (map[string]memory.ConfigSet, error) {
	var dcfg devConfig
	di := ds.Get(c)
	switch err := di.Get(&dcfg); err {
	case nil:
		break

	case ds.ErrNoSuchEntity:
		log.Infof(c, "Installing empty development configuration datastore entry.")

		if err := di.Put(&dcfg); err != nil {
			log.WithError(err).Warningf(c, "Failed to install empty development configuration.")
		}
		return nil, config.ErrNoConfig

	default:
		return nil, err
	}

	configSet, configPath := ServiceConfigPath(c)
	return map[string]memory.ConfigSet{
		configSet: {
			configPath: dcfg.Config,
		},
	}, nil
}
