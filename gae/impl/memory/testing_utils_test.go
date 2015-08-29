// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/cmpbin"
)

func init() {
	serializationDeterministic = true
	serialize.WritePropertyMapDeterministic = true
}

var NEXT_STR = "NEXT MARKER"
var NEXT = &NEXT_STR

// Use like:
//   pmap(
//     "prop", "val", 0, 100, NEXT,
//     "other", "val", 0, 100, NEXT,
//   )
//
func pmap(stuff ...interface{}) ds.PropertyMap {
	ret := ds.PropertyMap{}

	nom := func() interface{} {
		if len(stuff) > 0 {
			ret := stuff[0]
			stuff = stuff[1:]
			return ret
		}
		return nil
	}

	for len(stuff) > 0 {
		pname := nom().(string)
		if pname[0] == '$' || (strings.HasPrefix(pname, "__") && strings.HasSuffix(pname, "__")) {
			for len(stuff) > 0 && stuff[0] != NEXT {
				ret[pname] = append(ret[pname], propNI(nom()))
			}
		} else {
			for len(stuff) > 0 && stuff[0] != NEXT {
				ret[pname] = append(ret[pname], prop(nom()))
			}
		}
		nom()
	}

	return ret
}

func nq(kind_ns ...string) ds.Query {
	if len(kind_ns) == 2 {
		return &queryImpl{kind: kind_ns[0], ns: kind_ns[1]}
	} else if len(kind_ns) == 1 {
		return &queryImpl{kind: kind_ns[0], ns: "ns"}
	}
	return &queryImpl{kind: "Foo", ns: "ns"}
}

func indx(kind string, orders ...string) *ds.IndexDefinition {
	ancestor := false
	if kind[len(kind)-1] == '!' {
		ancestor = true
		kind = kind[:len(kind)-1]
	}
	ret := &ds.IndexDefinition{Kind: kind, Ancestor: ancestor}
	for _, o := range orders {
		dir := ds.ASCENDING
		if o[0] == '-' {
			dir = ds.DESCENDING
			o = o[1:]
		}
		ret.SortBy = append(ret.SortBy, ds.IndexColumn{Property: o, Direction: dir})
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
		return dskey.New(globalAppID, "ns", kind, x, 0, p)
	case int:
		return dskey.New(globalAppID, "ns", kind, "", int64(x), p)
	default:
		return dskey.New(globalAppID, "ns", kind, "invalid", 100, p)
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
			serialize.WriteTime(buf, x)
		case ds.Key:
			serialize.WriteKey(buf, serialize.WithoutContext, x)
		case *ds.IndexDefinition:
			serialize.WriteIndexDefinition(buf, *x)
		case ds.Property:
			serialize.WriteProperty(buf, serialize.WithoutContext, x)
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
