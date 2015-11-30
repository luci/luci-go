// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"fmt"
	"sync/atomic"
)

func ExampleWorkPool() {
	val := int32(0)
	WorkPool(16, func(workC chan<- func()) {
		for i := 0; i < 256; i++ {
			workC <- func() {
				atomic.AddInt32(&val, 1)
			}
		}
	})

	fmt.Printf("got: %d", val)
	// Output: got: 256
}
