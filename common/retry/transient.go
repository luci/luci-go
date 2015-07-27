// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

import (
	"time"

	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
)

// TransientOnly is an Iterator implementation that only retries errors if they
// are transient.
//
// (See errors.IsTransient).
type TransientOnly struct {
	Iterator // The wrapped Iterator.
}

// Next implements the Iterator interface.
func (i *TransientOnly) Next(ctx context.Context, err error) time.Duration {
	if !errors.IsTransient(err) {
		return Stop
	}
	return i.Iterator.Next(ctx, err)
}
