// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"
)

// Options is the set of options used to set up a Manager.
//
// The configuration is loaded from a svcconfig.Config protobuf.
type Options struct {
	// Config is the configuration service to load from.
	Config config.Interface

	// ConfigSet is the name of the ConfigSet to load.
	ConfigSet string
	// ConfigPath is the name of the ConfigPath to load.
	ConfigPath string

	// KillFunc, if not nil, will be called if a configuration change has been
	// detected.
	KillFunc func()

	// KillCheckInterval, if >0, starts a goroutine that polls every interval to
	// see if the configuration has changed. If it has, KillFunc will be invoked.
	KillCheckInterval time.Duration
}

func (o *Options) startConfigChangedPoller(c context.Context, hash string) {
	for {
		log.Fields{
			"timeout": o.KillCheckInterval,
		}.Debugf(c, "Entering kill check poll loop...")

		select {
		case <-c.Done():
			log.WithError(c.Err()).Debugf(c, "Context cancelled, shutting down kill poller.")
			return

		case <-clock.After(c, o.KillCheckInterval):
			log.Infof(c, "Kill check timeout triggered, reloading configuration...")

			cfg, err := o.getConfig(true)
			if err != nil {
				log.WithError(err).Warningf(c, "Failed to reload configuration.")
				continue
			}

			if cfg.ContentHash != hash {
				log.Fields{
					"currentHash": hash,
					"newHash":     cfg.ContentHash,
				}.Errorf(c, "Configuration content hash has changed.")
				o.runKillFunc()
				return
			}

			log.Fields{
				"currentHash": hash,
			}.Debugf(c, "Content hash matches.")
		}
	}
}

func (o *Options) runKillFunc() {
	if o.KillFunc != nil {
		o.KillFunc()
	}
}

func (o *Options) getConfig(hashOnly bool) (*config.Config, error) {
	return o.Config.GetConfig(o.ConfigSet, o.ConfigPath, hashOnly)
}

// Manager holds and exposes a service configuration.
//
// It can also periodically refresh that configuration and invoke a shutdown
// function if its content changes.
type Manager struct {
	o *Options

	cfg         *svcconfig.Config
	contentHash string
}

// NewManager generates a new Manager and loads the initial configuration.
func NewManager(c context.Context, o Options) (*Manager, error) {
	m := Manager{
		o: &o,
	}

	// Load the initial configuration.
	if err := m.reloadConfig(c); err != nil {
		return nil, err
	}

	if o.KillCheckInterval > 0 {
		go m.o.startConfigChangedPoller(c, m.contentHash)
	}

	return &m, nil
}

// Config returns the service configuration instance.
func (m *Manager) Config() *svcconfig.Config {
	return m.cfg
}

func (m *Manager) reloadConfig(c context.Context) error {
	cfg, err := m.o.getConfig(false)
	if err != nil {
		return err
	}

	scfg := svcconfig.Config{}
	if err := proto.UnmarshalText(cfg.Content, &scfg); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"hash":       cfg.ContentHash,
		}.Errorf(c, "Failed to unmarshal configuration.")
		return err
	}

	m.cfg = &scfg
	m.contentHash = cfg.ContentHash
	return nil
}
