// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"infra/gae/libs/wrapper"

	"golang.org/x/net/context"
)

type giContextKeyType int

var giContextKey giContextKeyType

func curGID(c context.Context) *globalInfoData {
	return c.Value(giContextKey).(*globalInfoData)
}

// useGI adds a wrapper.GlobalInfo context, accessible
// by wrapper.GetGI(c)
func useGI(c context.Context) context.Context {
	return wrapper.SetGIFactory(c, func(ic context.Context) wrapper.GlobalInfo {
		return &giImpl{wrapper.DummyGI(), curGID(ic), ic}
	})
}

type globalInfoData struct{ namespace string }

type giImpl struct {
	wrapper.GlobalInfo
	data *globalInfoData
	c    context.Context
}

var _ = wrapper.GlobalInfo((*giImpl)(nil))

func (gi *giImpl) Namespace(ns string) (context.Context, error) {
	return context.WithValue(gi.c, giContextKey, &globalInfoData{ns}), nil
}
