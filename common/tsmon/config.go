// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"encoding/json"
	"os"
	"runtime"
)

// config is the representation of a tsmon JSON config file.
type config struct {
	Endpoint        string `json:"endpoint"`
	Credentials     string `json:"credentials"`
	AutoGenHostname bool   `json:"autogen_hostname"`
}

// loadConfig loads a tsmon JSON config from a file.
func loadConfig(path string) (config, error) {
	var ret config

	file, err := os.Open(path)
	if err != nil {
		return ret, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err = decoder.Decode(&ret); err != nil {
		return ret, err
	}

	return ret, nil
}

func defaultConfigFilePath() string {
	if runtime.GOOS == "windows" {
		return "C:\\chrome-infra\\ts-mon.json"
	}
	return "/etc/chrome-infra/ts-mon.json"
}
