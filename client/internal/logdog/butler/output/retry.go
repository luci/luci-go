// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package output

import (
	"time"

	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
)

// DefaultRetryIterator returns a retry.Iterator configured with a default
// exponential backoff retry configuration.
func DefaultRetryIterator(context.Context) retry.Iterator {
	// TODO: Tune backoff parameters.
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:    (500 * time.Millisecond),
			Retries:  4,
			MaxTotal: 20 * time.Second,
		},
		Multiplier: 2.0,
	}
}
