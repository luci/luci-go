// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	"github.com/luci/gae/service/module"
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

	mod module.Interface
}

var _ module.Interface = (*modCounter)(nil)

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
	return module.AddFilters(c, func(ic context.Context, mod module.Interface) module.Interface {
		return &modCounter{state, mod}
	}), state
}
