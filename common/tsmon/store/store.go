// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package store contains code for storing and retreiving metrics.
package store

import (
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

// A Store is responsible for handling all metric data.
type Store interface {
	Register(m types.Metric) error
	Unregister(name string)

	Get(ctx context.Context, name string, fieldVals []interface{}) (value interface{}, err error)
	Set(ctx context.Context, name string, fieldVals []interface{}, value interface{}) error
	Incr(ctx context.Context, name string, fieldVals []interface{}, delta interface{}) error

	GetAll(ctx context.Context) []types.Cell

	ResetForUnittest()
}
