// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	"github.com/luci/gae/service/module"
)

type modState struct {
	*state

	module.Interface
}

func (m *modState) List() (ret []string, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.List()
		return
	})
	return
}

func (m *modState) NumInstances(mod, ver string) (ret int, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.NumInstances(mod, ver)
		return
	})
	return
}

func (m *modState) SetNumInstances(mod, ver string, instances int) error {
	return m.run(func() (err error) {
		return m.Interface.SetNumInstances(mod, ver, instances)
	})
}

func (m *modState) Versions(mod string) (ret []string, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.Versions(mod)
		return
	})
	return
}

func (m *modState) DefaultVersion(mod string) (ret string, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.DefaultVersion(mod)
		return
	})
	return
}

func (m *modState) Start(mod, ver string) error {
	return m.run(func() (err error) {
		return m.Interface.Start(mod, ver)
	})
}

func (m *modState) Stop(mod, ver string) error {
	return m.run(func() (err error) {
		return m.Interface.Stop(mod, ver)
	})
}

// FilterModule installs a featureBreaker module filter in the context.
func FilterModule(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return module.AddFilters(c, func(ic context.Context, i module.Interface) module.Interface {
		return &modState{state, i}
	}), state
}
