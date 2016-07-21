// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"path/filepath"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/proto/deploy"

	"github.com/mitchellh/go-homedir"
)

func loadConfig() (*deploy.UserConfig, error) {
	var cfg deploy.UserConfig

	path, err := homedir.Dir()
	if err != nil {
		configPath := filepath.Join(path, ".luci_deploy")
		switch err := unmarshalTextProtobuf(configPath, &cfg); {
		case err == nil, isNotExist(err):
			break

		default:
			return nil, errors.Annotate(err).Reason("failed to load config at [%(path)s]").D("path", configPath).Err()
		}
	}

	// Populate any undefined values with defaults.
	return &cfg, nil
}
