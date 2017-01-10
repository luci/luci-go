// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package backend

// Meta is backend metadata about a single configuration file.
type Meta struct {
	// ConfigSet is the item's config set.
	ConfigSet string
	// Path is the item's path within its config set.
	Path string

	// ContentHash is the content hash.
	ContentHash string
	// Revision is the revision string.
	Revision string
}
