// Copyright 2016 The LUCI Authors.
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

package featureBreaker

import (
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/module"
)

type modState struct {
	*state

	c context.Context
	module.RawInterface
}

func (m *modState) List() (ret []string, err error) {
	err = m.run(m.c, func() (err error) {
		ret, err = m.RawInterface.List()
		return
	})
	return
}

func (m *modState) NumInstances(mod, ver string) (ret int, err error) {
	err = m.run(m.c, func() (err error) {
		ret, err = m.RawInterface.NumInstances(mod, ver)
		return
	})
	return
}

func (m *modState) SetNumInstances(mod, ver string, instances int) error {
	return m.run(m.c, func() (err error) {
		return m.RawInterface.SetNumInstances(mod, ver, instances)
	})
}

func (m *modState) Versions(mod string) (ret []string, err error) {
	err = m.run(m.c, func() (err error) {
		ret, err = m.RawInterface.Versions(mod)
		return
	})
	return
}

func (m *modState) DefaultVersion(mod string) (ret string, err error) {
	err = m.run(m.c, func() (err error) {
		ret, err = m.RawInterface.DefaultVersion(mod)
		return
	})
	return
}

func (m *modState) Start(mod, ver string) error {
	return m.run(m.c, func() (err error) {
		return m.RawInterface.Start(mod, ver)
	})
}

func (m *modState) Stop(mod, ver string) error {
	return m.run(m.c, func() (err error) {
		return m.RawInterface.Stop(mod, ver)
	})
}

// FilterModule installs a featureBreaker module filter in the context.
func FilterModule(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return module.AddFilters(c, func(ic context.Context, i module.RawInterface) module.RawInterface {
		return &modState{state, ic, i}
	}), state
}
