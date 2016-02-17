// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/luci/luci-go/common/errors"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func ExampleWorkPool() {
	val := int32(0)
	err := WorkPool(16, func(workC chan<- func() error) {
		for i := 0; i < 256; i++ {
			workC <- func() error {
				atomic.AddInt32(&val, 1)
				return nil
			}
		}
	})

	if err != nil {
		fmt.Printf("Unexpected error: %s", err.Error())
	}

	fmt.Printf("got: %d", val)
	// Output: got: 256
}

func TestWorkPool(t *testing.T) {
	t.Parallel()

	Convey("When running WorkPool tests", t, func() {
		Convey("Various sized workpools execute their work successfully", func() {
			val := int32(0)

			Convey("single goroutine", func() {
				WorkPool(1, func(ch chan<- func() error) {
					for i := 0; i < 100; i++ {
						ch <- func() error { atomic.AddInt32(&val, 1); return nil }
					}
				})

				So(val, ShouldEqual, 100)
			})

			Convey("multiple goroutines", func() {
				WorkPool(10, func(ch chan<- func() error) {
					for i := 0; i < 100; i++ {
						ch <- func() error { atomic.AddInt32(&val, 1); return nil }
					}
				})

				So(val, ShouldEqual, 100)
			})

			Convey("more goroutines than jobs", func() {
				WorkPool(1000, func(ch chan<- func() error) {
					for i := 0; i < 100; i++ {
						ch <- func() error { atomic.AddInt32(&val, 1); return nil }
					}
				})

				So(val, ShouldEqual, 100)
			})
		})

		Convey("and testing error handling with a workpool size of 1", func() {
			e1 := errors.New("red fish")
			e2 := errors.New("blue fish")
			Convey("every job failing returns every error", func() {
				result := WorkPool(1, func(ch chan<- func() error) {
					ch <- func() error { return e1 }
					ch <- func() error { return e2 }
				})

				So(result, ShouldHaveLength, 2)
				So(result, ShouldContain, e1)
				So(result, ShouldContain, e2)
			})

			Convey("some jobs failing return those errors", func() {
				result := WorkPool(1, func(ch chan<- func() error) {
					ch <- func() error { return nil }
					ch <- func() error { return e1 }
					ch <- func() error { return nil }
					ch <- func() error { return e2 }
				})

				So(result, ShouldHaveLength, 2)
				So(result, ShouldContain, e1)
				So(result, ShouldContain, e2)
			})
		})

		Convey("and testing the worker number parameter", func() {
			started := make([]bool, 2)
			okToTest := make(chan struct{}, 1)
			gogo := make(chan int)
			quitting := make(chan struct{}, 1)

			e1 := errors.New("1 fish")
			e2 := errors.New("2 fish")

			Convey("2 jobs with 1 worker sequences correctly", func(c C) {
				err := WorkPool(1, func(ch chan<- func() error) {
					ch <- func() error {
						started[0] = true
						okToTest <- struct{}{}
						gogo <- 1
						quitting <- struct{}{}
						return e1
					}
					ch <- func() error {
						started[1] = true
						okToTest <- struct{}{}
						gogo <- 2
						return e2
					}

					<-okToTest
					c.So(started[0], ShouldBeTrue)
					// Only 1 worker, so the second function should not have started
					// yet.
					c.So(started[1], ShouldBeFalse)

					c.So(<-gogo, ShouldEqual, 1)
					<-quitting

					// First worker should have died.
					<-okToTest
					c.So(started[1], ShouldBeTrue)
					c.So(<-gogo, ShouldEqual, 2)
				})
				So(err, ShouldResemble, errors.MultiError{
					// Make sure they return in the right order.
					e1, e2,
				})
			})
		})
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	Convey("When using Run directly", t, func() {
		Convey("Ignore consumes the errors and blocks", func() {
			count := new(int32)
			Ignore(Run(make(Semaphore, 1), func(ch chan<- func() error) {
				for i := 0; i < 100; i++ {
					ch <- func() error {
						atomic.AddInt32(count, 1)
						return fmt.Errorf("whaaattt")
					}
				}
			}))
			So(*count, ShouldEqual, 100)
		})

		Convey("Must panics on the first error", func() {
			count := new(int32)
			So(func() {
				Must(Run(make(Semaphore, 1), func(ch chan<- func() error) {
					for i := 0; i < 100; i++ {
						i := i
						ch <- func() error {
							atomic.AddInt32(count, 1)
							return fmt.Errorf("whaaattt: %d", i)
						}
					}
				}))
			}, ShouldPanicLike, "whaaattt: 0")
			// Either:
			//   * the panic happened and we load count before ch is unblocked
			//   * the panic happened then ch(1) pushes and runs, then we load count
			// So count will either be 1 or 2, but never more or less.
			So(atomic.LoadInt32(count), ShouldBeBetweenOrEqual, 1, 2)
		})
	})
}
