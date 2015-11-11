// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"fmt"
	"sync/atomic"
	"testing"
)

func ExampleFanOutIn() {
	data := []int{1, 20}
	err := FanOutIn(func(ch chan<- func() error) {
		for _, d := range data {
			d := d
			ch <- func() error {
				if d > 10 {
					return fmt.Errorf("%d is over 10", d)
				}
				return nil
			}
		}
	})

	fmt.Printf("got: %q", err)
	// Output: got: "20 is over 10"
}

func TestRaciness(t *testing.T) {
	t.Parallel()

	val := int32(0)

	for i := 0; i < 100; i++ {
		FanOutIn(func(ch chan<- func() error) {
			ch <- func() error { atomic.AddInt32(&val, 1); return nil }
		})
	}

	if val != 100 {
		t.Error("val != 100, was", val)
	}
}
