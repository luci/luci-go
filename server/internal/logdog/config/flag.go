// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"errors"
	"flag"
	"net/http"
	"time"

	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock/clockflag"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/remote"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/transport"
	"golang.org/x/net/context"
)

// Flags is a set of command-line flags used to set up the process'
// configuration.
type Flags struct {
	// ConfigPath is the path within the ConfigSet of the configuration.
	//
	// If ConfigURL is empty, this will be interpreted as a local filesystem path
	// from which the configuration should be loaded.
	ConfigFilePath string

	// KillCheckInterval, if >0, starts a goroutine that polls every interval to
	// see if the configuration has changed.
	KillCheckInterval clockflag.Duration

	// RoundTripper, if not nil, is the http.RoundTripper that will be used to
	// fetch remote configurations.
	RoundTripper http.RoundTripper
}

// AddToFlagSet augments the supplied FlagSet with values for Flags.
func (f *Flags) AddToFlagSet(fs *flag.FlagSet) {
	fs.StringVar(&f.ConfigFilePath, "config-file-path", "",
		"If set, load the configuration protobuf from this path instead of the configuration service.")
	fs.Var(&f.KillCheckInterval, "config-kill-interval",
		"If non-zero, poll for configuration changes and kill the application if one is detected.")
}

// CoordinatorOptions returns an Options instance loaded from the supplied flags
// and Coordinator instance.
func (f *Flags) CoordinatorOptions(c context.Context, client services.ServicesClient) (*Options, error) {
	ccfg, err := client.GetConfig(c, &google.Empty{})
	if err != nil {
		return nil, err
	}

	// If a ConfigFilePath was specified, use a mock configuration service that
	// loads from a local file.
	var ci config.Interface
	if f.ConfigFilePath != "" {
		ci = &fileConfig{
			path:       f.ConfigFilePath,
			configSet:  ccfg.ConfigSet,
			configPath: ccfg.ConfigPath,
		}
	} else {
		if ccfg.ConfigServiceUrl == "" {
			return nil, errors.New("coordinator does not specify a config service")
		}
		if ccfg.ConfigSet == "" {
			return nil, errors.New("coordinator does not specify a config set")
		}
		if ccfg.ConfigPath == "" {
			return nil, errors.New("coordinator does not specify a config path")
		}

		if f.RoundTripper != nil {
			c = transport.Set(c, f.RoundTripper)
		}
		ci = remote.New(c, ccfg.ConfigServiceUrl)
	}

	return &Options{
		Config:            ci,
		ConfigSet:         ccfg.ConfigSet,
		ConfigPath:        ccfg.ConfigPath,
		KillCheckInterval: time.Duration(f.KillCheckInterval),
	}, nil
}
