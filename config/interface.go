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

package config

import (
	"context"
	"errors"
	"net/url"
)

// ErrNoConfig is returned if requested config does not exist.
var ErrNoConfig = errors.New("no such config")

// Meta is metadata about a single configuration file.
type Meta struct {
	// ConfigSet is the config set name (e.g. "projects/<id>") this config
	// belongs to.
	ConfigSet Set `json:"configSet,omitempty"`

	// Path is the filename relative to the root of the config set,
	// without leading slash, e.g. "luci-scheduler.cfg".
	Path string `json:"path,omitempty"`

	// ContentHash can be used to quickly check that content didn't change.
	ContentHash string `json:"contentHash,omitempty"`

	// Revision is git SHA1 of a repository the config was fetched from.
	Revision string `json:"revision,omitempty"`

	// ViewURL is the URL surfaced for viewing the config.
	ViewURL string `json:"view_url,omitempty"`
}

// Config is a configuration file along with its metadata.
type Config struct {
	Meta

	// Error is not nil if there where troubles fetching this config. Used only
	// by functions that operate with multiple configs at once, such as
	// GetProjectConfigs.
	Error error `json:"error,omitempty"`

	// Content is the actual body of the config file.
	Content string `json:"content,omitempty"`
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
	// ID is unique project identifier.
	ID string

	// Name is a short friendly display name of the project.
	// In remote_v2 implementation, Name and ID are always the same.
	Name string

	// RepoType specifies in what kind of storage projects configs are stored.
	RepoType RepoType

	// RepoUrl is the location of this project code repository. May be nil if
	// unknown or cannot be parsed.
	RepoURL *url.URL
}

// Interface represents low-level pull-based LUCI Config API.
//
// This is roughly a wrapper over LUCI Config RPC interface, and all methods
// are generally slow and depend on available of LUCI Config service. They
// *must not* be used in a hot path of requests.
//
// Transient errors are tagged with transient.Tag.
type Interface interface {
	// GetConfig returns a config at a path in a config set or ErrNoConfig
	// if missing. If metaOnly is true, returned Config struct has only Meta set
	// (and the call is faster).
	GetConfig(ctx context.Context, configSet Set, path string, metaOnly bool) (*Config, error)

	// GetConfigs returns a bunch of config files fetched at the same revision.
	// If filter is nil, will return all files in the config set. Otherwise only
	// files that pass the filter are returned. The filter is called sequentially
	// in the same goroutine as GetConfigs itself and it receives paths in some
	// arbitrary order. If metaOnly is true, returned Config structs have only
	// Meta set (and the call is much faster). If there's no such config set at
	// all, returns ErrNoConfig.
	GetConfigs(ctx context.Context, configSet Set, filter func(path string) bool, metaOnly bool) (map[string]Config, error)

	// GetProjectConfigs returns all the configs at the given path in all
	// projects that have such config. If metaOnly is true, returned Config
	// structs have only Meta set (and the call is faster).
	GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]Config, error)

	// GetProjects returns all the registered projects in the configuration
	// service.
	GetProjects(ctx context.Context) ([]Project, error)

	// ListFiles returns the list of files for a config set.
	ListFiles(ctx context.Context, configSet Set) ([]string, error)

	// Close closes resources the config.Interface uses.
	// The caller is expected to call Close() after the config.Interface is no
	// longer used.
	Close() error
}
