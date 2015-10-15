// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package render

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
)

// StringDeep converts a structure to a string representation. Unline the "%#v"
// format string, this resolves pointer types' contents in structs, maps, and
// slices/arrays and prints their field values.
func StringDeep(v interface{}) string {
	buf := bytes.Buffer{}
	s := (*traverseState)(nil)
	s.stringDeep(&buf, 0, reflect.ValueOf(v))
	return buf.String()
}

// renderPointer is overridable by the test suite in order to have deterministic
// pointer values.
var renderPointer = func(buf *bytes.Buffer, p uintptr) {
	fmt.Fprintf(buf, "0x%016x", p)
}

// traverseState is used to note and avoid recursion as struct members are being
// traversed.
//
// traverseState is allowed to be nil. Specifically, the root state is nil.
type traverseState struct {
	parent *traverseState
	ptr    uintptr
}

func (s *traverseState) forkFor(ptr uintptr) *traverseState {
	for cur := s; cur != nil; cur = cur.parent {
		if ptr == cur.ptr {
			return nil
		}
	}

	fs := &traverseState{
		parent: s,
		ptr:    ptr,
	}
	return fs
}

func (s *traverseState) stringDeep(buf *bytes.Buffer, ptrs int, v reflect.Value) {
	vt := v.Type()

	// If the type being rendered is a potentially recursive type (a type that
	// can contain itself as a member), we need to avoid recursion.
	//
	// If we've already seen this type before, mark that this is the case and
	// write a recursion placeholder instead of actually rendering it.
	//
	// If we haven't seen it before, fork our `seen` tracking so any higher-up
	// renderers will also render it at least once, then mark that we've seen it
	// to avoid recursing on lower layers.
	pe := uintptr(0)
	switch vt.Kind() {
	case reflect.Ptr:
		// Since structs and arrays aren't pointers, they can't directly be
		// recursed, but they can contain pointers to themselves. Record their
		// pointer to avoid this.
		switch v.Elem().Kind() {
		case reflect.Struct, reflect.Array:
			pe = v.Pointer()
		}

	case reflect.Slice, reflect.Map:
		pe = v.Pointer()
	}
	if pe != 0 {
		s = s.forkFor(pe)
		if s == nil {
			buf.WriteString("<REC")
			writeType(buf, ptrs, true, vt)
			buf.WriteRune('>')
			return
		}
	}

	switch vt.Kind() {
	case reflect.Struct:
		writeType(buf, ptrs, false, vt)
		buf.WriteRune('{')
		for i := 0; i < vt.NumField(); i++ {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(vt.Field(i).Name)
			buf.WriteRune(':')

			s.stringDeep(buf, 0, v.Field(i))
		}
		buf.WriteRune('}')

	case reflect.Array, reflect.Slice:
		writeType(buf, ptrs, false, vt)
		buf.WriteString("{")
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				buf.WriteString(", ")
			}

			s.stringDeep(buf, 0, v.Index(i))
		}
		buf.WriteRune('}')

	case reflect.Map:
		writeType(buf, ptrs, false, vt)
		buf.WriteString("{")

		mkeys := v.MapKeys()
		tryAndSortMapKeys(vt, mkeys)

		for i, mk := range mkeys {
			if i > 0 {
				buf.WriteString(", ")
			}

			s.stringDeep(buf, 0, mk)
			buf.WriteString(": ")
			s.stringDeep(buf, 0, v.MapIndex(mk))
		}
		buf.WriteRune('}')

	case reflect.Ptr:
		ptrs++
		fallthrough
	case reflect.Interface:
		if v.IsNil() {
			fmt.Fprint(buf, "nil")
		} else {
			s.stringDeep(buf, ptrs, v.Elem())
		}

	case reflect.Bool:
		typedFPrintf(buf, ptrs, vt, "%v", v.Bool())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		typedFPrintf(buf, ptrs, vt, "%d", v.Int())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		typedFPrintf(buf, ptrs, vt, "%d", v.Uint())

	case reflect.Float32, reflect.Float64:
		typedFPrintf(buf, ptrs, vt, "%g", v.Float())

	case reflect.Complex64, reflect.Complex128:
		typedFPrintf(buf, ptrs, vt, "%g", v.Complex())

	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		writeType(buf, ptrs, false, vt)
		buf.WriteRune('(')
		renderPointer(buf, v.Pointer())
		buf.WriteRune(')')

	case reflect.String:
		typedFPrintf(buf, ptrs, vt, "%q", v.String())

	default:
		typedFPrintf(buf, ptrs, vt, "%v", v)
	}
}

func writeType(buf *bytes.Buffer, ptrs int, forceParens bool, t reflect.Type) {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		forceParens = true
	}

	if forceParens || ptrs > 0 {
		buf.WriteRune('(')
		for i := 0; i < ptrs; i++ {
			buf.WriteRune('*')
		}
	}

	buf.WriteString(t.String())

	if forceParens || ptrs > 0 {
		buf.WriteRune(')')
	}
}

func typedFPrintf(buf *bytes.Buffer, ptrs int, t reflect.Type, f string, args ...interface{}) {
	if ptrs > 0 {
		writeType(buf, ptrs, false, t)
		buf.WriteRune('(')
	}

	fmt.Fprintf(buf, f, args...)

	if ptrs > 0 {
		buf.WriteRune(')')
	}
}

type sortableValueSlice struct {
	kind     reflect.Kind
	elements []reflect.Value
}

func (s *sortableValueSlice) Len() int {
	return len(s.elements)
}

func (s *sortableValueSlice) Less(i, j int) bool {
	switch s.kind {
	case reflect.String:
		return s.elements[i].String() < s.elements[j].String()

	case reflect.Int:
		return s.elements[i].Int() < s.elements[j].Int()

	default:
		panic(fmt.Errorf("unsupported sort kind: %s", s.kind))
	}
}

func (s *sortableValueSlice) Swap(i, j int) {
	s.elements[i], s.elements[j] = s.elements[j], s.elements[i]
}

func tryAndSortMapKeys(mt reflect.Type, k []reflect.Value) {
	// Try our stock sortable values.
	switch mt.Key().Kind() {
	case reflect.String, reflect.Int:
		vs := &sortableValueSlice{
			kind:     mt.Key().Kind(),
			elements: k,
		}
		sort.Sort(vs)
	}
}
