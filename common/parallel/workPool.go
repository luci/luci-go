// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package parallel

import (
	"github.com/luci/luci-go/common/errors"
)

// WorkPool creates a fixed-size pool of worker goroutines. A supplied generator
// method creates task functions and passes them through to the work pool.
//
// WorkPool will use at most workers goroutines to execute the supplied tasks.
// If workers is <= 0, WorkPool will be unbounded and behave like FanOutIn.
//
// WorkPool blocks until all the generator completes and all workers have
// finished their tasks.
func WorkPool(workers int, gen func(chan<- func() error)) error {
	return errors.MultiErrorFromErrors(Run(workers, gen))
}
