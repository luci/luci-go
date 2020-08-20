// Copyright 2020 The LUCI Authors.
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

package cas

import (
	"flag"
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

type Flags struct {
	Instance string

	TokenServerHost string
}

func (c *Flags) Init(f *flag.FlagSet) {
	f.StringVar(&c.Instance, "cas-instance", "", "CAS instance (GCP). Format is either a project ID, or \"projects/<project_id>/instances/<instance_id>\"")

	f.StringVar(&c.TokenServerHost, "token-server-host", chromeinfra.TokenServerHost, "LUCI token server used to get access token for CAS instance")
}

func (c *Flags) Parse() error {
	if c.Instance == "" {
		// -cas-instance is optional for now
		return nil
	}
	ins, err := parseCASInstance(c.Instance)
	if err != nil {
		return err
	}
	c.Instance = ins
	return nil
}

func parseCASInstance(ins string) (string, error) {
	// GCP project ID format: https://cloud.google.com/resource-manager/docs/creating-managing-projects
	// Not the most accurate regexp, but let's just assume most people know what they are doing...
	projectRe := regexp.MustCompile(`^[a-z0-9\-]+$`)
	instanceRe := regexp.MustCompile(`^projects/[a-z0-9\-]+/instances/[^/]+$`)

	if projectRe.MatchString(ins) {
		return "projects/" + ins + "/instances/default_instance", nil
	}
	if instanceRe.MatchString(ins) {
		return ins, nil
	}
	return "", errors.Reason("invalid CAS instance: %s", ins).Err()
}
