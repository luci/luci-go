// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"github.com/luci/luci-go/common/errors"
)

// Normalize returns an error iff the WalkGraphReq is invalid.
func (w *WalkGraphReq) Normalize() error {
	if len(w.Queries) == 0 {
		return errors.New("must specify at least one Query")
	}

	lme := errors.NewLazyMultiError(len(w.Queries))
	for i, q := range w.Queries {
		lme.Assign(i, q.Normalize())
	}
	if err := lme.Get(); err != nil {
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

	if w.Include == nil {
		w.Include = &WalkGraphReq_Include{}
	} else {
		if w.Include.AttemptResult {
			w.Include.AttemptData = true
		}
	}
	return nil
}
