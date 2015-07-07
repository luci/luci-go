// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"reflect"
	"time"

	"appengine/datastore"

	"github.com/luci/luci-go/common/cmpbin"
)

type kv struct{ k, v []byte }

func indx(kind string, orders ...string) *qIndex {
	ancestor := false
	if kind[len(kind)-1] == '!' {
		ancestor = true
		kind = kind[:len(kind)-1]
	}
	ret := &qIndex{kind, ancestor, nil}
	for _, o := range orders {
		dir := qASC
		if o[0] == '-' {
			dir = qDEC
			o = o[1:]
		}
		ret.sortby = append(ret.sortby, qSortBy{o, dir})
	}
	return ret
}

func pl(props ...datastore.Property) *propertyList {
	return (*propertyList)(&props)
}

func prop(name string, val interface{}, noIndex ...bool) (ret datastore.Property) {
	ret.Name = name
	ret.Value = val
	if len(noIndex) > 0 {
		ret.NoIndex = noIndex[0]
	}
	return
}

func key(kind string, id interface{}, parent ...*datastore.Key) *datastore.Key {
	stringID := ""
	intID := int64(0)
	switch x := id.(type) {
	case string:
		stringID = x
	case int:
		intID = int64(x)
	default:
		panic(fmt.Errorf("what the %T: %v", id, id))
	}
	par := (*datastore.Key)(nil)
	if len(parent) > 0 {
		par = parent[0]
	}
	return newKey("ns", kind, stringID, intID, par)
}

func mustLoadLocation(loc string) *time.Location {
	if z, err := time.LoadLocation(loc); err != nil {
		panic(err)
	} else {
		return z
	}
}

// cat is a convenience method for concatenating anything with an underlying
// byte representation into a single []byte.
func cat(bytethings ...interface{}) []byte {
	buf := &bytes.Buffer{}
	for _, thing := range bytethings {
		switch x := thing.(type) {
		case int, int64:
			cmpbin.WriteInt(buf, reflect.ValueOf(x).Int())
		case uint, uint64:
			cmpbin.WriteUint(buf, reflect.ValueOf(x).Uint())
		case float64:
			writeFloat64(buf, x)
		case byte, propValType:
			buf.WriteByte(byte(reflect.ValueOf(x).Uint()))
		case []byte, serializedPval:
			buf.Write(reflect.ValueOf(x).Convert(byteSliceType).Interface().([]byte))
		case string:
			writeString(buf, x)
		case time.Time:
			writeTime(buf, x)
		case *datastore.Key:
			writeKey(buf, noNS, x)
		case *qIndex:
			x.WriteBinary(buf)
		default:
			panic(fmt.Errorf("I don't know how to deal with %T: %#v", thing, thing))
		}
	}
	ret := buf.Bytes()
	if ret == nil {
		ret = []byte{}
	}
	return ret
}

func icat(bytethings ...interface{}) []byte {
	ret := cat(bytethings...)
	for i := range ret {
		ret[i] ^= 0xFF
	}
	return ret
}

func sat(bytethings ...interface{}) string {
	return string(cat(bytethings...))
}
