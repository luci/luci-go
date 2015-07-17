// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"errors"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type otherMEType []error

func (o otherMEType) Error() string { return "FAIL" }

func TestUpstreamErrors(t *testing.T) {
	t.Parallel()

	Convey("Test MultiError", t, func() {
		Convey("nil", func() {
			me := MultiError(nil)
			So(me.Error(), ShouldEqual, "(0 errors)")
			Convey("single", func() {
				So(SingleError(error(me)), ShouldBeNil)
			})
		})
		Convey("one", func() {
			me := MultiError{errors.New("sup")}
			So(me.Error(), ShouldEqual, "sup")
		})
		Convey("two", func() {
			me := MultiError{errors.New("sup"), errors.New("what")}
			So(me.Error(), ShouldEqual, "sup (and 1 other error)")
		})
		Convey("more", func() {
			me := MultiError{errors.New("sup"), errors.New("what"), errors.New("nerds")}
			So(me.Error(), ShouldEqual, "sup (and 2 other errors)")

			Convey("single", func() {
				So(SingleError(error(me)), ShouldResemble, errors.New("sup"))
			})
		})
		Convey("convert", func() {
			ome := otherMEType{errors.New("sup")}
			So(FixError(ome), ShouldHaveSameTypeAs, MultiError{})
		})
	})

	Convey("FixError passes through", t, func() {
		e := errors.New("unique")
		So(FixError(e), ShouldEqual, e)
	})

	Convey("SingleError passes through", t, func() {
		e := errors.New("unique")
		So(SingleError(e), ShouldEqual, e)
	})
}

func TestLazyMultiError(t *testing.T) {
	t.Parallel()

	Convey("Test LazyMultiError", t, func() {
		lme := LazyMultiError{Size: 10}
		So(lme.Get(), ShouldEqual, nil)

		e := errors.New("sup")
		lme.Assign(6, e)
		So(lme.Get(), ShouldResemble,
			MultiError{nil, nil, nil, nil, nil, nil, e, nil, nil, nil})

		lme.Assign(2, e)
		So(lme.Get(), ShouldResemble,
			MultiError{nil, nil, e, nil, nil, nil, e, nil, nil, nil})

		So(func() { lme.Assign(20, e) }, ShouldPanic)

		Convey("Try to freak out the race detector", func() {
			lme := LazyMultiError{Size: 64}
			Convey("all nils", func() {
				wg := sync.WaitGroup{}
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func(i int) {
						lme.Assign(i, nil)
						wg.Done()
					}(i)
				}
				wg.Wait()
				So(lme.Get(), ShouldBeNil)
			})
			Convey("every other", func() {
				wow := errors.New("wow")
				wg := sync.WaitGroup{}
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func(i int) {
						e := error(nil)
						if i&1 == 1 {
							e = wow
						}
						lme.Assign(i, e)
						wg.Done()
					}(i)
				}
				wg.Wait()
				me := make(MultiError, 64)
				for i := range me {
					if i&1 == 1 {
						me[i] = wow
					}
				}
				So(lme.Get(), ShouldResemble, me)
			})
			Convey("all", func() {
				wow := errors.New("wow")
				wg := sync.WaitGroup{}
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func(i int) {
						lme.Assign(i, wow)
						wg.Done()
					}(i)
				}
				wg.Wait()
				me := make(MultiError, 64)
				for i := range me {
					me[i] = wow
				}
				So(lme.Get(), ShouldResemble, me)
			})
		})
	})
}
