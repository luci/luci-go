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

// Config provides some methods for interacting with the config.
type Config interface {
	// HasHost returns whether the host is configured.
	HasHost(host string) bool
	// ShouldIndexRepo returns whether the specified repository should be indexed.
	ShouldIndexRepo(host, repo string) bool
	// ShouldIndexRef returns whether the specified ref should be indexed.
	ShouldIndexRef(host, repo, ref string) bool
	// HostConfigs returns a list of host configs.
	HostConfigs() []HostConfig
}

// config is a wrapper around `*configpb.Config` that implements `Config`.
type config struct {
	*configpb.Config
}

// HasHost implements Config.
func (c *config) HasHost(host string) bool {
	return slices.ContainsFunc(c.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
}

// ShouldIndexRepo implements Config.
func (c config) ShouldIndexRepo(host, repo string) bool {
	hostIndex := slices.IndexFunc(c.Hosts, func(hostConfig *configpb.Config_Host) bool {
		return hostConfig.Host == host
	})
	if hostIndex < 0 {
		return false
	}
	hostConfig := c.Hosts[hostIndex]

	return slices.ContainsFunc(hostConfig.Repositories, func(repoConfig *configpb.Config_Host_Repository) bool {
		return repoConfig.Name == repo
	})
}

// ShouldIndexRef implements Config.
func (c config) ShouldIndexRef(host, repo, ref string) bool {
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

	return repositoryConfig{repoConfig}.ShouldIndexRef(ref)
}

// HostConfigs implements Config.
func (c config) HostConfigs() []HostConfig {
	hostConfigs := make([]HostConfig, 0, len(c.Hosts))
	for _, host := range c.Hosts {
		hostConfigs = append(hostConfigs, hostConfig{host})
	}
	return hostConfigs
}

// HostConfig provides some methods for interacting with the host config.
type HostConfig interface {
	// Host returns the host name.
	Host() string
	// RepoConfigs returns a list of repository configs.
	RepoConfigs() []RepositoryConfig
}

// hostConfig is a wrapper around `*configpb.Config_Host` that implements
// `HostConfig`.
type hostConfig struct {
	*configpb.Config_Host
}

// Host implements HostConfig.
func (hc hostConfig) Host() string {
	return hc.GetHost()
}

// RepoConfigs implements HostConfig.
func (hc hostConfig) RepoConfigs() []RepositoryConfig {
	repoConfigs := make([]RepositoryConfig, 0, len(hc.Repositories))
	for _, repo := range hc.Repositories {
		repoConfigs = append(repoConfigs, repositoryConfig{repo})
	}
	return repoConfigs
}

// RepositoryConfig provides some methods for interacting with the repository
// config.
type RepositoryConfig interface {
	// Name returns the name of the repository.
	Name() string
	// ShouldIndexRef returns whether the specified ref should be indexed.
	ShouldIndexRef(ref string) bool
}

// hostConfig is a wrapper around `*configpb.Config_Host_Repository` that
// implements `RepositoryConfig`.
type repositoryConfig struct {
	*configpb.Config_Host_Repository
}

// Name implements Config.
func (rc repositoryConfig) Name() string {
	return rc.GetName()
}

// ShouldIndexRef implements Config.
func (rc repositoryConfig) ShouldIndexRef(ref string) bool {
	matchAnyPrefix := slices.ContainsFunc(IndexableRefPrefixes, func(prefix string) bool {
		return strings.HasPrefix(ref, prefix)
	})
	if !matchAnyPrefix {
		return false
	}

	return slices.ContainsFunc(rc.IncludeRefRegexes, func(regexStr string) bool {
		regex := regexp.MustCompile("^" + regexStr + "$")
		return regex.MatchString(ref)
	})
}
