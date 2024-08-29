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
	"strings"

	configpb "go.chromium.org/luci/source_index/proto/config"
)

// IndexableRefPrefixes is a list of ref prefixes from which the commits can be
// indexed.
//
// We should not index commits of arbitrary refs (e.g. Gerrit CL patch refs)
// as they may not have been merged into one of the branches. We don't want to
// resolve commit positions to unmerged CLs.
//
// Keep in sync with the service config documentation
// `luci.source_index.config.Config.Host.Repository.include_ref_regexes`
var IndexableRefPrefixes = []string{
	"refs/branch-heads/",
	"refs/heads/",
}

// Config is a wrapper around `*configpb.Config` that  provides some methods for
// interacting with the Config.
type Config struct {
	cfg *configpb.Config
}

// HasHost returns whether the host is configured.
func (c *Config) HasHost(host string) bool {
	return slices.ContainsFunc(c.cfg.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
}

// ShouldIndexRepo returns whether the specified repository should be indexed.
func (c Config) ShouldIndexRepo(host, repo string) bool {
	hostIndex := slices.IndexFunc(c.cfg.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
	if hostIndex < 0 {
		return false
	}
	hostConfig := c.cfg.Hosts[hostIndex]

	return slices.ContainsFunc(hostConfig.Repositories, func(repoConfig *configpb.Config_Host_Repository) bool {
		return repoConfig.Name == repo
	})
}

// ShouldIndexRef returns whether the specified ref should be indexed.
func (c Config) ShouldIndexRef(host, repo, ref string) bool {
	hostIndex := slices.IndexFunc(c.cfg.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
	if hostIndex < 0 {
		return false
	}
	hostConfig := c.cfg.Hosts[hostIndex]

	repoIndex := slices.IndexFunc(hostConfig.Repositories, func(repoConfig *configpb.Config_Host_Repository) bool {
		return repoConfig.Name == repo
	})
	if repoIndex < 0 {
		return false
	}
	repoConfig := hostConfig.Repositories[repoIndex]

	return RepositoryConfig{repoConfig}.ShouldIndexRef(ref)
}

// HostConfigs returns a list of host configs.
func (c Config) HostConfigs() []HostConfig {
	hostConfigs := make([]HostConfig, 0, len(c.cfg.Hosts))
	for _, host := range c.cfg.Hosts {
		hostConfigs = append(hostConfigs, HostConfig{host})
	}
	return hostConfigs
}

// HostConfig is a wrapper around `*configpb.Config_Host` that provides some
// methods for interacting with the host config.
type HostConfig struct {
	cfg *configpb.Config_Host
}

// Host returns the host name.
func (hc HostConfig) Host() string {
	return hc.cfg.GetHost()
}

// RepoConfigs returns a list of repository configs.
func (hc HostConfig) RepoConfigs() []RepositoryConfig {
	repoConfigs := make([]RepositoryConfig, 0, len(hc.cfg.Repositories))
	for _, repo := range hc.cfg.Repositories {
		repoConfigs = append(repoConfigs, RepositoryConfig{repo})
	}
	return repoConfigs
}

// RepositoryConfig is a wrapper around `*configpb.Config_Host_Repository` that
// provides some methods for interacting with the repository config.
type RepositoryConfig struct {
	cfg *configpb.Config_Host_Repository
}

// Name returns the name of the repository.
func (rc RepositoryConfig) Name() string {
	return rc.cfg.GetName()
}

// ShouldIndexRef returns whether the specified ref should be indexed.
func (rc RepositoryConfig) ShouldIndexRef(ref string) bool {
	matchAnyPrefix := slices.ContainsFunc(IndexableRefPrefixes, func(prefix string) bool {
		return strings.HasPrefix(ref, prefix)
	})
	if !matchAnyPrefix {
		return false
	}

	return slices.ContainsFunc(rc.cfg.IncludeRefRegexes, func(regexStr string) bool {
		regex := regexp.MustCompile("^" + regexStr + "$")
		return regex.MatchString(ref)
	})
}
