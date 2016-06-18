// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dm

import (
	"math"

	"github.com/luci/luci-go/common/errors"
)

const (
	// DefaultLimitMaxDataSize is the default MaxDataSize value (16MB).
	DefaultLimitMaxDataSize = 16 * 1024 * 1024

	// MaxLimitMaxDataSize is the maximum MaxDataSize value (30MB).
	MaxLimitMaxDataSize = 30 * 1024 * 1024
)

// MakeWalkGraphIncludeAll makes a new WalkGraphReq_Include which has all the
// boxes ticked. This should only be used when your application plans to dump
// the resulting graph query data to some logging/debugging trace for humans.
//
// If you don't plan on dumping it for humans, please set the Include options
// appropriately in order to avoid wasting bandwidth/cpu/datastore query time on
// the server (and draining your DM quotas unnecessarially).
func MakeWalkGraphIncludeAll() *WalkGraphReq_Include {
	return &WalkGraphReq_Include{
		true, true, true, true, true, math.MaxUint32, true, true, true,
	}
}

// Normalize returns an error iff the WalkGraphReq is invalid.
func (w *WalkGraphReq) Normalize() error {
	if w.Auth != nil {
		if err := w.Auth.Normalize(); err != nil {
			return err
		}
	}

	if w.Query == nil {
		return errors.New("must specify a Query")
	}
	if err := w.Query.Normalize(); err != nil {
		return err
	}

	if w.Mode == nil {
		w.Mode = &WalkGraphReq_Mode{}
	}

	if w.Limit != nil {
		if w.Limit.MaxDepth < -1 {
			return errors.New("Limit.MaxDepth must be >= -1")
		}
		if w.Limit.GetMaxTime().Duration() < 0 {
			return errors.New("Limit.MaxTime must be positive")
		}
	} else {
		w.Limit = &WalkGraphReq_Limit{}
	}
	if w.Limit.MaxDataSize == 0 {
		w.Limit.MaxDataSize = DefaultLimitMaxDataSize
	}
	if w.Limit.MaxDataSize > MaxLimitMaxDataSize {
		w.Limit.MaxDataSize = MaxLimitMaxDataSize
	}

	if w.Include == nil {
		w.Include = &WalkGraphReq_Include{}
	} else {
		if w.Include.AttemptResult {
			w.Include.AttemptData = true
		}
		if w.Include.NumExecutions == 0 {
			w.Include.ExecutionInfoUrl = false
		}
	}
	return nil
}
