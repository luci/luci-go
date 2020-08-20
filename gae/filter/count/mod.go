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

package count

import (
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/module"
)

// ModuleCounter is the counter object for the Module service.
type ModuleCounter struct {
	List            Entry
	NumInstances    Entry
	SetNumInstances Entry
	Versions        Entry
	DefaultVersion  Entry
	Start           Entry
	Stop            Entry
}

type modCounter struct {
	c *ModuleCounter

	mod module.RawInterface
}

var _ module.RawInterface = (*modCounter)(nil)

func (m *modCounter) List() ([]string, error) {
	ret, err := m.mod.List()
	return ret, m.c.List.up(err)
}

func (m *modCounter) NumInstances(mod, ver string) (int, error) {
	ret, err := m.mod.NumInstances(mod, ver)
	return ret, m.c.NumInstances.up(err)
}

func (m *modCounter) SetNumInstances(mod, ver string, instances int) error {
	return m.c.SetNumInstances.up(m.mod.SetNumInstances(mod, ver, instances))
}

func (m *modCounter) Versions(mod string) ([]string, error) {
	ret, err := m.mod.Versions(mod)
	return ret, m.c.Versions.up(err)
}

func (m *modCounter) DefaultVersion(mod string) (string, error) {
	ret, err := m.mod.DefaultVersion(mod)
	return ret, m.c.DefaultVersion.up(err)
}

func (m *modCounter) Start(mod, ver string) error {
	return m.c.Start.up(m.mod.Start(mod, ver))
}

func (m *modCounter) Stop(mod, ver string) error {
	return m.c.Stop.up(m.mod.Stop(mod, ver))
}

// FilterModule installs a counter Module filter in the context.
func FilterModule(c context.Context) (context.Context, *ModuleCounter) {
	state := &ModuleCounter{}
	return module.AddFilters(c, func(ic context.Context, mod module.RawInterface) module.RawInterface {
		return &modCounter{state, mod}
	}), state
}
