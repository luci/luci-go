// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"net/url"
)

// Config is a configuration entry in the luci-config service.
type Config struct {
	// ConfigSet is the config set this config belongs to.
	// May be the empty string if this is unknown.
	ConfigSet string

	Content string

	ContentHash string

	Revision string
}

// RepoType is the type of the repo the Project is stored in.
type RepoType string

const (
	// GitilesRepo means a repo is backed by the Gitiles service.
	GitilesRepo RepoType = "GITILES"

	// UnknownRepo means a repo is backed by an unknown service.
	// It may be an invalid repo.
	UnknownRepo RepoType = "UNKNOWN"
)

// Project is a project registered in the luci-config service.
type Project struct {
	ID string

	Name string

	RepoType RepoType

	// RepoUrl is the location of this project code repository.
	RepoURL *url.URL
}

// Interface is the interface any client has to access the luci-config service.
type Interface interface {
	// GetConfig returns a config at a path in a config set.
	GetConfig(configSet, path string) (*Config, error)

	// GetConfigByHash returns the contents of a config, as identified by its content hash.
	GetConfigByHash(contentHash string) (string, error)

	// GetConfigSetLocation returns the URL location of a config set.
	GetConfigSetLocation(configSet string) (*url.URL, error)

	// GetProjectConfigs returns all the configs at the given path in all projects.
	GetProjectConfigs(path string) ([]*Config, error)

	// GetProjects returns all the registered projects in the configuration service.
	GetProjects() ([]*Project, error)

	// GetRefConfigs returns the config at the given path in all refs of all projects.
	GetRefConfigs(path string) ([]*Config, error)

	// GetRefs returns the list of refs for a project.
	GetRefs(projectID string) ([]string, error)
}
