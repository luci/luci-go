// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type foo struct {
	BrokenFeatures
}

func (f *foo) RunIfNotBroken(fn func() error) error {
	// can 'override' RunIfNotBroken
	return f.BrokenFeatures.RunIfNotBroken(fn)
}

func (f *foo) Foo() (ret string, err error) {
	err = f.RunIfNotBroken(func() error {
		ret = "foo"
		return nil
	})
	return
}

func (f *foo) Bar() (ret string, err error) {
	err = f.RunIfNotBroken(func() error {
		ret = "bar"
		return nil
	})
	return
}

type override struct {
	BrokenFeatures
	totallyRekt bool
}

func (o *override) RunIfNotBroken(f func() error) error {
	if o.totallyRekt {
		return fmt.Errorf("totallyRekt")
	}
	return o.BrokenFeatures.RunIfNotBroken(f)
}

func (o *override) Foo() error {
	return o.RunIfNotBroken(func() error { return nil })
}

func TestBrokenFeatures(t *testing.T) {
	e := errors.New("sup")
	eCustom := fmt.Errorf("bad stuff happened")
	f := foo{BrokenFeatures{DefaultError: e}}

	Convey("BrokenFeatures", t, func() {
		Convey("can break functions", func() {
			s, err := f.Foo()
			So(s, ShouldEqual, "foo")
			So(err, ShouldBeNil)

			f.BreakFeatures(nil, "Foo")
			_, err = f.Foo()
			So(err, ShouldEqual, e)

			Convey("and unbreak them", func() {
				f.UnbreakFeatures("Foo")
				s, err = f.Foo()
				So(s, ShouldEqual, "foo")
				So(err, ShouldBeNil)
			})

			Convey("and breaking features doesn't break unrelated ones", func() {
				s, err := f.Bar()
				So(s, ShouldEqual, "bar")
				So(err, ShouldBeNil)
			})
		})

		Convey("Can override IsBroken too", func() {
			o := &override{BrokenFeatures{DefaultError: e}, false}
			Convey("Can break functions as normal", func() {
				o.BreakFeatures(nil, "Foo")
				So(o.Foo(), ShouldEqual, e)

				Convey("but can also break them in a user defined way", func() {
					o.totallyRekt = true
					So(o.Foo().Error(), ShouldContainSubstring, "totallyRekt")
				})
			})
		})

		Convey("Not specifying a default gets you a generic error", func() {
			f.BrokenFeatures.DefaultError = nil
			f.BreakFeatures(nil, "Foo")
			_, err := f.Foo()
			So(err.Error(), ShouldContainSubstring, `"Foo"`)
		})

		Convey("Can override the error returned", func() {
			f.BreakFeatures(eCustom, "Foo")
			v, err := f.Foo()
			So(v, ShouldEqual, "")
			So(err, ShouldEqual, eCustom)
		})

		Convey("Can be broken if not embedded", func(c C) {
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				bf := BrokenFeatures{DefaultError: e}
				// break some feature so we're forced to crawl the stack.
				bf.BreakFeatures(nil, "Nerds")
				// should break because there's no exported functions on the stack.
				err := bf.RunIfNotBroken(func() error { return nil })
				c.So(err, ShouldEqual, ErrBrokenFeaturesBroken)
			}()
			wg.Wait()
		})
	})
}
