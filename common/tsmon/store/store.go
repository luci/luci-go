// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package store contains code for storing and retreiving metrics.
package store

import (
	"time"

	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

// Modification contains all the arguments for a Set or Incr call.
type Modification struct {
	Metric    types.Metric
	ResetTime time.Time
	FieldVals []interface{}
	Target    types.Target

	SetValue  interface{} // nil unless this is a Set.
	IncrDelta interface{} // nil unless this is an Incr.
}

// A Store is responsible for handling all metric data.
type Store interface {
	Register(m types.Metric)
	Unregister(m types.Metric)

	Get(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}) (value interface{}, err error)
	Set(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}, value interface{}) error
	Incr(ctx context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}, delta interface{}) error
	ModifyMulti(ctx context.Context, mods []Modification) error

	GetAll(ctx context.Context) []types.Cell

	ResetForUnittest()
}
