// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

import (
	"time"

	"golang.org/x/net/context"
)

// defaultIterator defines a template for the default retry parameters that
// should be used throughout the program.
var defaultIteratorTemplate = ExponentialBackoff{
	Limited: Limited{
		Delay:   200 * time.Millisecond,
		Retries: 10,
	},
	MaxDelay:   10 * time.Second,
	Multiplier: 2,
}

// Default is a Factory that returns a new instance of the default iterator
// configuration.
func Default(context.Context) Iterator {
	return &defaultIteratorTemplate
}
