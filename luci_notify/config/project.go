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
	"fmt"

	"go.chromium.org/gae/service/datastore"
	configInterface "go.chromium.org/luci/common/config"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

// LuciConfigURL returns a user friendly URL that specifies where to view
// this console definition.
func luciConfigURL(c context.Context, configSet, path, revision string) string {
	// TODO(mknyszek): This shouldn't be hardcoded, instead we should get the
	// luci-config instance from the context.
	// TODO(mknyszek): The UI doesn't allow specifying paths and revision yet.  Add
	// that in when it is supported.
	return fmt.Sprintf("https://luci-config.appspot.com/newui#/%s", configSet)
}

// Project represents the luci-notify configuration for a single project in the datastore.
type Project struct {
	// Name is the name of the project.
	//
	// This must be unique on this luci-notify instance.
	Name string `gae:"$id"`

	// The revision of this project's luci-notify configuration.
	Revision string

	// The luci-config URL to this project's luci-notify configuration.
	URL string
}

// NewProject constructs a new Project from a name and a luci-config configuration.
func NewProject(c context.Context, name string, cfg *configInterface.Config) *Project {
	return &Project{
		Name:     name,
		Revision: cfg.Revision,
		URL:      luciConfigURL(c, cfg.ConfigSet, cfg.Path, cfg.Revision),
	}
}

// GetProject queries the datastore for a single Project.
func GetProject(c context.Context, project string) (*Project, error) {
	p := Project{Name: project}
	err := datastore.Get(c, &p)
	return &p, err
}

// UpdateDatastore puts the Project into the datastore.
func (p *Project) UpdateDatastore(c context.Context) error {
	if err := datastore.Put(c, p); err != nil {
		return errors.Annotate(err, "saving %s", p.Name).Err()
	}
	return nil
}
