// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/golang/protobuf/proto"
	yaml "gopkg.in/yaml.v2"

	apackpb "go.chromium.org/luci/apack/proto"
	"go.chromium.org/luci/common/errors"
)

func readConfigFile(filePath string) (*apackpb.Config, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	contents, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	cfg := &apackpb.Config{}
	if err := proto.UnmarshalText(string(contents), cfg); err != nil {
		return nil, err
	}

	if cfg.AppConfig == "" {
		cfg.AppConfig = filepath.Join(filepath.Dir(filePath), "app.yaml")
	}

	// TODO: validate

	return cfg, nil
}

var reGSPath = regexp.MustCompile("^gs://([^/]+)/(.+)$")

func parseGSPath(path string) (bucket, object string, err error) {
	m := reGSPath.FindStringSubmatch(path)
	if m == nil {
		return "", "", errors.Reason("%q does not match regexp %q", path, reGSPath.String()).Err()
	}

	return m[1], m[2], nil
}

type serviceConfig struct {
	SkipFiles []string `yaml:"skip_files"`
}

func parseServiceConfig(filename string) (*serviceConfig, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &serviceConfig{}
	return cfg, yaml.NewDecoder(f).Decode(cfg)
}
