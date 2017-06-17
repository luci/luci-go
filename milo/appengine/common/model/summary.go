// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

import "time"

// Summary summarizes a thing (step, build, group of builds, whatever).
type Summary struct {
	// Status indicates the 'goodness' and lifetime of the thing. This usually
	// translates directly to a status color.
	Status Status

	// Start indicates when this thing started doing its action.
	Start time.Time

	// End indicates when this thing completed doing its action.
	End time.Time

	// Text is a possibly-multi-line summary of what happened.
	Text []string
}
