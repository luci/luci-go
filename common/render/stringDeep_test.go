// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package render

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	renderPointer = func(buf *bytes.Buffer, p uintptr) {
		buf.WriteString("PTR")
	}
}

func TestStringDeep(t *testing.T) {
	// Note that we make the fields exportable. This is to avoid a fun case where
	// the first reflect.Value has a read-only bit set, but follow-on values
	// do not, so recursion tests are off by one.
	type testStruct struct {
		Name string
		I    interface{}
	}

	s0 := "string0"
	s0P := &s0
	stringer := fmt.Stringer(nil)

	Convey(`Test cases`, t, func() {
		for i, tc := range []struct {
			a interface{}
			s string
		}{
			{nil, `nil`},
			{make(chan int), `(chan int)(PTR)`},
			{&stringer, `(*fmt.Stringer)(nil)`},
			{123, `123`},
			{"hello", `"hello"`},
			{testStruct{Name: "foo", I: &testStruct{Name: "baz"}},
				`render.testStruct{Name:"foo", I:(*render.testStruct){Name:"baz", I:interface{}(nil)}}`},
			{[]byte(nil), `[]uint8(nil)`},
			{[]byte{}, `[]uint8{}`},
			{map[string]string(nil), `map[string]string(nil)`},
			{[]*testStruct{
				{Name: "foo"},
				{Name: "bar"},
			}, `[]*render.testStruct{(*render.testStruct){Name:"foo", I:interface{}(nil)}, ` +
				`(*render.testStruct){Name:"bar", I:interface{}(nil)}}`},
			{struct {
				a int
				b string
			}{123, "foo"}, `struct { a int; b string }{a:123, b:"foo"}`},
			{[]string{"foo", "foo", "bar", "baz", "qux", "qux"},
				`[]string{"foo", "foo", "bar", "baz", "qux", "qux"}`},
			{[...]int{1, 2, 3}, `[3]int{1, 2, 3}`},
			{map[string]bool{
				"foo": true,
				"bar": false,
			}, `map[string]bool{"bar": false, "foo": true}`},
			{map[int]string{1: "foo", 2: "bar"}, `map[int]string{1: "foo", 2: "bar"}`},
			{uint32(1337), `1337`},
			{3.14, `3.14`},
			{complex(3, 0.14), `(3+0.14i)`},
			{&s0, `(*string)("string0")`},
			{&s0P, `(**string)("string0")`},
			{[]interface{}{nil, 1, 2, nil}, `[]interface{}{interface{}(nil), 1, 2, interface{}(nil)}`},
		} {
			Convey(fmt.Sprintf(`%d: string of [%s] is %q`, i, reflect.ValueOf(tc.a), tc.s), func() {
				So(StringDeep(tc.a), ShouldEqual, tc.s)
			})
		}
	})

	Convey(`A recursive struct will note the recursion and stop.`, t, func() {
		s := &testStruct{
			Name: "recursive",
		}
		s.I = s
		So(StringDeep(s), ShouldEqual,
			`(*render.testStruct){Name:"recursive", I:<REC(*render.testStruct)>}`)
	})

	Convey(`A recursive array will note the recursion and stop.`, t, func() {
		a := [2]interface{}{}
		a[0] = &a
		a[1] = &a

		So(StringDeep(&a), ShouldEqual,
			`(*[2]interface{}){<REC(*[2]interface{})>, <REC(*[2]interface{})>}`)
	})

	Convey(`A recursive map will note the recursion and stop.`, t, func() {
		m := map[string]interface{}{}
		foo := "foo"
		m["foo"] = m
		m["bar"] = [](*string){&foo, &foo}
		v := []map[string]interface{}{m, m}

		So(StringDeep(v), ShouldEqual,
			`[]map[string]interface{}{map[string]interface{}{`+
				`"bar": []*string{(*string)("foo"), (*string)("foo")}, `+
				`"foo": <REC(map[string]interface{})>}, `+
				`map[string]interface{}{`+
				`"bar": []*string{(*string)("foo"), (*string)("foo")}, `+
				`"foo": <REC(map[string]interface{})>}}`)
	})
}
