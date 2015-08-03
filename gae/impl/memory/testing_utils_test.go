// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"time"

	ds "github.com/luci/gae/service/datastore"
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

var (
	prop   = ds.MkProperty
	propNI = ds.MkPropertyNI
)

func key(kind string, id interface{}, parent ...ds.Key) ds.Key {
	p := ds.Key(nil)
	if len(parent) > 0 {
		p = parent[0]
	}
	switch x := id.(type) {
	case string:
		return ds.NewKey(globalAppID, "ns", kind, x, 0, p)
	case int:
		return ds.NewKey(globalAppID, "ns", kind, "", int64(x), p)
	default:
		panic(fmt.Errorf("what the %T: %v", id, id))
	}
}

// cat is a convenience method for concatenating anything with an underlying
// byte representation into a single []byte.
func cat(bytethings ...interface{}) []byte {
	buf := &bytes.Buffer{}
	for _, thing := range bytethings {
		switch x := thing.(type) {
		case int64:
			cmpbin.WriteInt(buf, x)
		case int:
			cmpbin.WriteInt(buf, int64(x))
		case uint64:
			cmpbin.WriteUint(buf, x)
		case uint:
			cmpbin.WriteUint(buf, uint64(x))
		case float64:
			cmpbin.WriteFloat64(buf, x)
		case byte:
			buf.WriteByte(x)
		case ds.PropertyType:
			buf.WriteByte(byte(x))
		case string:
			cmpbin.WriteString(buf, x)
		case []byte:
			buf.Write(x)
		case time.Time:
			ds.WriteTime(buf, x)
		case ds.Key:
			ds.WriteKey(buf, ds.WithoutContext, x)
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
