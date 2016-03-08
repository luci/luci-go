// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"github.com/luci/gae/service/module"
	"golang.org/x/net/context"
)

type modContextKeyType int

var modContextKey modContextKeyType

type moduleVersion struct {
	module, version string
}

type modImpl struct {
	c            context.Context
	numInstances map[moduleVersion]int
}

// useMod adds a Module interface to the context
func useMod(c context.Context) context.Context {
	return module.SetFactory(c, func(ic context.Context) module.Interface {
		return &modImpl{ic, map[moduleVersion]int{}}
	})
}

var _ = module.Interface((*modImpl)(nil))

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

func (mod *modImpl) Start(module, version string) error {
	return nil
}

func (mod *modImpl) Stop(module, version string) error {
	return nil
}
