// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"sync"

	"infra/libs/errors"
)

// FanOutIn is useful to quickly parallelize a group of tasks.
//
// You pass it a function which is expected to push simple `func() error`
// closures into the provided chan. Each function will be executed in parallel
// and their error results will be collated.
//
// The function blocks until all functions are executed, and an
// errors.MultiError is returned if one or more of your fan-out tasks failed,
// otherwise this function returns nil.
func FanOutIn(gen func(chan<- func() error)) error {
	funchan := make(chan func() error)
	go func() {
		defer close(funchan)
		gen(funchan)
	}()

	errchan := make(chan error)
	grp := sync.WaitGroup{}
	for fn := range funchan {
		grp.Add(1)
		fn := fn
		go func() {
			defer grp.Done()
			if err := fn(); err != nil {
				errchan <- err
			}
		}()
	}
	go func() {
		grp.Wait()
		close(errchan)
	}()
	return errors.MultiErrorFromErrors(errchan)
}
