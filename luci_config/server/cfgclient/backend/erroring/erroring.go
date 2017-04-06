// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package erroring implements config.Backend that simply returns an error.
//
// May be handy as a placeholder in case some more useful implementation is not
// available.
package erroring

import (
	"errors"
	"net/url"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"

	"golang.org/x/net/context"
)

// New produces backend.B instance that returns the given error for all calls.
//
// Panics if given err is nil.
func New(err error) backend.B {
	if err == nil {
		panic("the error must not be nil")
	}
	return erroringImpl{err}
}

type erroringImpl struct {
	err error
}

func (e erroringImpl) ServiceURL(context.Context) url.URL {
	return url.URL{
		Scheme: "error",
	}
}

func (e erroringImpl) Get(c context.Context, configSet, path string, p backend.Params) (*backend.Item, error) {
	return nil, e.err
}

func (e erroringImpl) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) (
	[]*backend.Item, error) {

	return nil, e.err
}

func (e erroringImpl) ConfigSetURL(c context.Context, configSet string, p backend.Params) (url.URL, error) {
	return url.URL{}, e.err
}

func (e erroringImpl) GetConfigInterface(c context.Context, a backend.Authority) config.Interface {
	emptySet := map[string]memory.ConfigSet{}
	i := memory.New(emptySet)
	memory.SetError(i, errors.New("error"))
	return i
}
