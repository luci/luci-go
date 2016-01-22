// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

// wrap wraps a Factory, applying a modification function to each Iterator
// that it produces.
func wrap(g Factory, mod func(it Iterator) Iterator) Factory {
	return func() Iterator {
		next := g()
		if next == nil {
			return nil
		}
		return mod(next)
	}
}
