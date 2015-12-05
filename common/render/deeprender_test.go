// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package render

import (
	"bytes"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	renderPointer = func(buf *bytes.Buffer, p uintptr) {
		buf.WriteString("PTR")
	}
}

func TestDeepRender(t *testing.T) {
	// Note that we make some of the fields exportable. This is to avoid a fun case
	// where the first reflect.Value has a read-only bit set, but follow-on values
	// do not, so recursion tests are off by one.
	type testStruct struct {
		Name string
		I    interface{}

		m string
	}

	type myStringSlice []string
	type myStringMap map[string]string
	type myIntType int
	type myStringType string

	Convey(`Deep render test suite`, t, func() {

		s0 := "string0"
		s0P := &s0
		mit := myIntType(42)
		stringer := fmt.Stringer(nil)

		for _, tc := range []struct {
			a interface{}
			s string
		}{
			{nil, `nil`},
			{make(chan int), `(chan int)(PTR)`},
			{&stringer, `(*fmt.Stringer)(nil)`},
			{123, `123`},
			{"hello", `"hello"`},
			{(*testStruct)(nil), `(*render.testStruct)(nil)`},
			{(**testStruct)(nil), `(**render.testStruct)(nil)`},
			{[]***testStruct(nil), `[]***render.testStruct(nil)`},
			{testStruct{Name: "foo", I: &testStruct{Name: "baz"}},
				`render.testStruct{Name:"foo", I:(*render.testStruct){Name:"baz", I:interface{}(nil), m:""}, m:""}`},
			{[]byte(nil), `[]uint8(nil)`},
			{[]byte{}, `[]uint8{}`},
			{map[string]string(nil), `map[string]string(nil)`},
			{[]*testStruct{
				{Name: "foo"},
				{Name: "bar"},
			}, `[]*render.testStruct{(*render.testStruct){Name:"foo", I:interface{}(nil), m:""}, ` +
				`(*render.testStruct){Name:"bar", I:interface{}(nil), m:""}}`},
			{myStringSlice{"foo", "bar"}, `render.myStringSlice{"foo", "bar"}`},
			{myStringMap{"foo": "bar"}, `render.myStringMap{"foo":"bar"}`},
			{myIntType(12), `render.myIntType(12)`},
			{&mit, `(*render.myIntType)(42)`},
			{myStringType("foo"), `render.myStringType("foo")`},
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
			}, `map[string]bool{"bar":false, "foo":true}`},
			{map[int]string{1: "foo", 2: "bar"}, `map[int]string{1:"foo", 2:"bar"}`},
			{uint32(1337), `1337`},
			{3.14, `3.14`},
			{complex(3, 0.14), `(3+0.14i)`},
			{&s0, `(*string)("string0")`},
			{&s0P, `(**string)("string0")`},
			{[]interface{}{nil, 1, 2, nil}, `[]interface{}{interface{}(nil), 1, 2, interface{}(nil)}`},
		} {
			Convey(fmt.Sprintf(`Can properly render %q`, tc.s), func() {
				So(DeepRender(tc.a), ShouldEqual, tc.s)
			})
		}

		// Recursive struct.
		s := &testStruct{
			Name: "recursive",
		}
		s.I = s
		So(DeepRender(s), ShouldEqual,
			`(*render.testStruct){Name:"recursive", I:<REC(*render.testStruct)>, m:""}`)

		// Recursive array.
		a := [2]interface{}{}
		a[0] = &a
		a[1] = &a

		So(DeepRender(&a), ShouldEqual,
			`(*[2]interface{}){<REC(*[2]interface{})>, <REC(*[2]interface{})>}`)

		// Recursive map.
		m := map[string]interface{}{}
		foo := "foo"
		m["foo"] = m
		m["bar"] = [](*string){&foo, &foo}
		v := []map[string]interface{}{m, m}

		So(DeepRender(v), ShouldEqual,
			`[]map[string]interface{}{map[string]interface{}{`+
				`"bar":[]*string{(*string)("foo"), (*string)("foo")}, `+
				`"foo":<REC(map[string]interface{})>}, `+
				`map[string]interface{}{`+
				`"bar":[]*string{(*string)("foo"), (*string)("foo")}, `+
				`"foo":<REC(map[string]interface{})>}}`)
	})
}
