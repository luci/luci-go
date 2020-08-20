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

package memory

import (
	"go.chromium.org/gae/service/module"
	"golang.org/x/net/context"
)

type modContextKeyType int

var modContextKey modContextKeyType

type moduleVersion struct {
	module, version string
}

type modImpl struct {
	numInstances map[moduleVersion]int
}

// useMod adds a Module interface to the context
func useMod(c context.Context) context.Context {
	modMap := map[moduleVersion]int{}
	return module.SetFactory(c, func(ic context.Context) module.RawInterface {
		return &modImpl{modMap}
	})
}

var _ = module.RawInterface((*modImpl)(nil))

func (mod *modImpl) List() ([]string, error) {
	return []string{"testModule1", "testModule2"}, nil
}

func (mod *modImpl) NumInstances(module, version string) (int, error) {
	if ret, ok := mod.numInstances[moduleVersion{module, version}]; ok {
		return ret, nil
	}
	return 1, nil
}

func (mod *modImpl) SetNumInstances(module, version string, instances int) error {
	mod.numInstances[moduleVersion{module, version}] = instances
	return nil
}

func (mod *modImpl) Versions(module string) ([]string, error) {
	return []string{"testVersion1", "testVersion2"}, nil
}

func (mod *modImpl) DefaultVersion(module string) (string, error) {
	return "testVersion1", nil
}

func (mod *modImpl) Start(module, version string) error { return nil }
func (mod *modImpl) Stop(module, version string) error  { return nil }
