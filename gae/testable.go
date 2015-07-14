// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

// Testable is the basic interface that every fake service should implement.
type Testable interface {
	FeatureBreaker
}
