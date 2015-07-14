// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

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

func (f *foo) halp() error { // test the ability to call IsBroken from an internal helper
	return f.IsBroken()
}

func (f *foo) Foo() (string, error) {
	err := f.halp()
	if err != nil {
		return "", err
	}
	return "foo", nil
}

func (f *foo) Bar() (string, error) {
	err := f.halp()
	if err != nil {
		return "", err
	}
	return "bar", nil
}

type override struct {
	BrokenFeatures
	totallyRekt bool
}

func (o *override) IsBroken() error {
	if o.totallyRekt {
		return fmt.Errorf("totallyRekt")
	}
	return o.BrokenFeatures.IsBroken()
}

func (o *override) Foo() error {
	return o.IsBroken()
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
				c.So(bf.IsBroken(), ShouldEqual, ErrBrokenFeaturesBroken)
			}()
			wg.Wait()
		})
	})
}
