// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"fmt"
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

	fmt.Printf("got: %s", err)
	// Output: got: ["20 is over 10"]
}
