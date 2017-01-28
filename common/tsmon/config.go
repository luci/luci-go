// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tsmon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/luci/luci-go/common/errors"
	homedir "github.com/mitchellh/go-homedir"
)

// config is the representation of a tsmon JSON config file.
type config struct {
	Endpoint        string `json:"endpoint"`
	Credentials     string `json:"credentials"`
	AutoGenHostname bool   `json:"autogen_hostname"`
	Hostname        string `json:"hostname"`
	Region          string `json:"region"`
}

// loadConfig loads a tsmon JSON config from a file.
func loadConfig(path string) (config, error) {
	var ret config

	if path == "" {
		return ret, nil
	}

	file, err := os.Open(path)
	switch {
	case err == nil:
		defer file.Close()

		decoder := json.NewDecoder(file)
		if err = decoder.Decode(&ret); err != nil {
			return ret, errors.Annotate(err).Reason("failed to decode file").Err()
		}
		return ret, nil

	case os.IsNotExist(err):
		// The file does not exist. We don't consider this an error, since the file
		// is optional.
		return ret, nil

	default:
		// An unexpected failure occurred.
		return ret, errors.Annotate(err).Reason("failed to open file").Err()
	}
}

func defaultConfigFilePath() string {
	if runtime.GOOS == "windows" {
		return "C:\\chrome-infra\\ts-mon.json"
	}
	return "/etc/chrome-infra/ts-mon.json"
}

func tokenCachePath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "chrome_infra", "auth"), nil
}
