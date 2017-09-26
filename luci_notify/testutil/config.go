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

package testutil

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"

	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
)

// LoadProjectConfig returns a luci-notify configuration from api/config/testdata
// by filename (without extension). It does NOT validate the configuration.
func LoadProjectConfig(filename string) (*notifyConfig.ProjectConfig, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fullname := fmt.Sprintf("%s/../api/config/testdata/%s.cfg", dir, filename)
	buf, err := ioutil.ReadFile(fullname)
	if err != nil {
		return nil, err
	}
	return ParseProjectConfig(string(buf[:]))
}

// ParseProjectConfig parses a luci-notify configuration from a string. It does
// NOT validate the configuration.
func ParseProjectConfig(config string) (*notifyConfig.ProjectConfig, error) {
	project := notifyConfig.ProjectConfig{}
	if err := proto.UnmarshalText(config, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ParseSettings parses a luci-notify service configuration from a string. It does
// NOT validate the configuration.
func ParseSettings(config string) (*notifyConfig.Settings, error) {
	settings := notifyConfig.Settings{}
	if err := proto.UnmarshalText(config, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}
