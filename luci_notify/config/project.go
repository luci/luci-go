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

package config

import (
	configInterface "go.chromium.org/luci/common/config"
)

// Project represents the luci-notify configuration for a single project in the datastore.
type Project struct {
	// Name is the name of the project.
	//
	// This must be unique on this luci-notify instance.
	Name string `gae:"$id"`

	// Revision is the revision of this project's luci-notify configuration.
	Revision string

	// URL is the luci-config URL to this project's luci-notify configuration.
	URL string
}

// NewProject constructs a new Project from a name and a luci-config configuration.
func NewProject(name string, cfg *configInterface.Config) *Project {
	return &Project{
		Name:     name,
		Revision: cfg.Revision,
		URL:      cfg.ViewURL,
	}
}
