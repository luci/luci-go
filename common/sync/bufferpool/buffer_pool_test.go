// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bufferpool

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBufferPool(t *testing.T) {
	t.Parallel()

	Convey(`Testing P, the buffer pool`, t, func() {
		var p P

		Convey(`Basic functionality`, func() {
			buf1 := p.Get()
			buf1.WriteString("foo")
			So(buf1.String(), ShouldEqual, "foo")

			clone1 := buf1.Clone()
			So(clone1, ShouldResemble, []byte("foo"))
			buf1.Release()

			buf2 := p.Get()
			buf2.WriteString("bar")

			// clone1 should still resemble "foo".
			So(clone1, ShouldResemble, []byte("foo"))
		})

		Convey(`Double release panicks`, func() {
			buf := p.Get()
			buf.Release()
			So(buf.Release, ShouldPanic)
		})

		Convey(`Concurrent access`, func() {
			const goroutines = 16
			const rounds = 128

			startC := make(chan struct{})
			doneC := make(chan []byte)

			for i := 0; i < goroutines; i++ {
				go func(idx int) {
					<-startC

					for j := 0; j < rounds; j++ {
						buf := p.Get()
						fmt.Fprintf(buf, "%d.%d", idx, j)
						doneC <- buf.Clone()
						buf.Release()
					}
				}(i)
			}

			// Collect all of our data. Store it as bytes so that if there is
			// conflict / reuse, something will hopefully go wrong.
			close(startC)
			data := make([][]byte, 0, goroutines*rounds)
			for i := 0; i < goroutines; i++ {
				for j := 0; j < rounds; j++ {
					data = append(data, <-doneC)
				}
			}

			// Assert that it all exists.
			sorted := make(map[string]struct{}, len(data))
			for _, d := range data {
				sorted[string(d)] = struct{}{}
			}
			So(len(sorted), ShouldEqual, goroutines*rounds)
		})
	})
}
