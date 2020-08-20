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

package prod

import (
	"go.chromium.org/gae/service/module"
	"golang.org/x/net/context"
	aeModule "google.golang.org/appengine/module"
)

// useModule adds a Module implementation to context.
func useModule(usrCtx context.Context) context.Context {
	return module.SetFactory(usrCtx, func(ci context.Context) module.RawInterface {
		return modImpl{getAEContext(ci)}
	})
}

type modImpl struct {
	aeCtx context.Context
}

func (m modImpl) List() ([]string, error) {
	return aeModule.List(m.aeCtx)
}

func (m modImpl) NumInstances(module, version string) (int, error) {
	return aeModule.NumInstances(m.aeCtx, module, version)
}

func (m modImpl) SetNumInstances(module, version string, instances int) error {
	return aeModule.SetNumInstances(m.aeCtx, module, version, instances)
}

func (m modImpl) Versions(module string) ([]string, error) {
	return aeModule.Versions(m.aeCtx, module)
}

func (m modImpl) DefaultVersion(module string) (string, error) {
	return aeModule.DefaultVersion(m.aeCtx, module)
}

func (m modImpl) Start(module, version string) error {
	return aeModule.Start(m.aeCtx, module, version)
}

func (m modImpl) Stop(module, version string) error {
	return aeModule.Stop(m.aeCtx, module, version)
}
