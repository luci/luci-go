// Copyright 2024 The LUCI Authors.
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

// Package config implements service-level config for LUCI Source Index.
package config

import (
	"regexp"
	"slices"

	configpb "go.chromium.org/luci/source_index/proto/config"
)

// config is a wrapper around `*configpb.Config` that implements `Config`.
type config struct {
	*configpb.Config
}

// Config provides some methods for interacting with the config.
type Config interface {
	// HasHost returns whether the host is configured.
	HasHost(host string) bool
	// ShouldIndexRef returns whether the specified ref should be indexed.
	ShouldIndexRef(host, repo, ref string) bool
}

// HasHost implements Config.
func (c *config) HasHost(host string) bool {
	return slices.ContainsFunc(c.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
}

// ShouldIndexRef implements Config.
func (c *config) ShouldIndexRef(host, repo, ref string) bool {
	hostIndex := slices.IndexFunc(c.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
	if hostIndex < 0 {
		return false
	}
	hostConfig := c.Hosts[hostIndex]

	repoIndex := slices.IndexFunc(hostConfig.Repositories, func(repoConfig *configpb.Config_Host_Repository) bool {
		return repoConfig.Name == repo
	})
	if repoIndex < 0 {
		return false
	}
	repoConfig := hostConfig.Repositories[repoIndex]

	return slices.ContainsFunc(repoConfig.IncludeRefRegexes, func(regexStr string) bool {
		regex := regexp.MustCompile(regexStr)
		return regex.MatchString(ref)
	})
}
