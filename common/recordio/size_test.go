// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package recordio

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFrameHeaderSize(t *testing.T) {
	t.Parallel()

	Convey(`Testing FrameHeaderSize`, t, func() {
		prev := -1
		for i := 0; i < 5; i++ {
			base := 1 << uint(7*i)
			for delta := range []int{-1, 0, 1} {
				base += delta
				if base <= prev {
					continue
				}
				prev = base

				Convey(fmt.Sprintf(`Is accurate for buffer size %d.`, base), func() {
					data := bytes.Repeat([]byte{0x55}, base)

					amt, err := WriteFrame(ioutil.Discard, data)
					if err != nil {
						panic(err)
					}

					So(amt-len(data), ShouldEqual, FrameHeaderSize(int64(len(data))))
				})
			}
		}
	})
}
