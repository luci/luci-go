// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

// FanOutIn is useful to quickly parallelize a group of tasks.
//
// You pass it a function which is expected to push simple `func() error`
// closures into the provided chan. Each function will be executed in parallel
// and their error results will be collated.
//
// The function blocks until all functions are executed, and an
// errors.MultiError is returned if one or more of your fan-out tasks failed,
// otherwise this function returns nil.
//
// This function is equivalent to WorkPool(0, gen).
func FanOutIn(gen func(chan<- func() error)) error {
	return Run(nil, gen)
}
