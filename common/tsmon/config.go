// Copyright 2015 The LUCI Authors.
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

package tsmon

import (
	"encoding/json"
	"os"
	"runtime"

	"go.chromium.org/luci/common/errors"
)

// config is the representation of a tsmon JSON config file.
type config struct {
	Endpoint        string `json:"endpoint"`
	Credentials     string `json:"credentials"`
	ActAs           string `json:"act_as"`
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
			return ret, errors.Fmt("failed to decode file: %w", err)
		}
		return ret, nil

	case os.IsNotExist(err):
		// The file does not exist. We don't consider this an error, since the file
		// is optional.
		return ret, nil

	default:
		// An unexpected failure occurred.
		return ret, errors.Fmt("failed to open file: %w", err)
	}
}

func defaultConfigFilePath() string {
	// TODO(vadimsh): Move this to "hardcoded/chromeinfra" package.
	if runtime.GOOS == "windows" {
		return "C:\\chrome-infra\\ts-mon.json"
	}
	return "/etc/chrome-infra/ts-mon.json"
}
