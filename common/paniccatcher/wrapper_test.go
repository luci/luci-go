// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package paniccatcher

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestCatch(t *testing.T) {
	Convey(`Do will suppress a panic.`, t, func() {
		didPanic := false
		So(func() {
			didPanic = Do(context.Background(), func() {
				panic("Everybody panic!")
			})
		}, ShouldNotPanic)
		So(didPanic, ShouldBeTrue)
	})

	Convey(`Wrapper can catch a panic.`, t, func() {
		w := Wrapper{}
		e := errors.New("TEST ERROR")
		func() {
			defer w.Catch(context.Background(), "Caught panic")
			panic(e)
		}()
		So(w.DidPanic(), ShouldBeTrue)
		So(w.Panic, ShouldEqual, e)
		So(string(w.Stack), ShouldContainSubstring, "TestCatch")
	})
}

// Example is a very simple example of how to use Catch to recover from a panic
// and log its stack trace.
func Example() {
	ctx := context.Background()

	Do(ctx, func() {
		// Do something?
		panic("Something wrong happened!")
	})
}
