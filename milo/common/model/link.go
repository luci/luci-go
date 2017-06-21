// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

// Link denotes a single labeled link.
type Link struct {
	// Title (text) of the link.
	Label string

	// The destination for the link.
	URL string
}
