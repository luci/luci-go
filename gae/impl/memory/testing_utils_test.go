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
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/cmpbin"
)

func init() {
	serializationDeterministic = true
	serialize.WritePropertyMapDeterministic = true
}

var nextMarker = "NEXT MARKER"
var Next = &nextMarker

// Use like:
//   pmap(
//     "prop", "val", 0, 100, Next,
//     "other", "val", 0, 100, Next,
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
			for len(stuff) > 0 && stuff[0] != Next {
				ret[pname] = append(ret[pname], propNI(nom()))
			}
		} else {
			for len(stuff) > 0 && stuff[0] != Next {
				ret[pname] = append(ret[pname], prop(nom()))
			}
		}
		nom()
	}

	return ret
}

func nq(kindMaybe ...string) *ds.Query {
	kind := "Foo"
	if len(kindMaybe) == 1 {
		kind = kindMaybe[0]
	}
	return ds.NewQuery(kind)
}

func indx(kind string, orders ...string) *ds.IndexDefinition {
	ancestor := false
	if kind[len(kind)-1] == '!' {
		ancestor = true
		kind = kind[:len(kind)-1]
	}
	ret := &ds.IndexDefinition{Kind: kind, Ancestor: ancestor}
	for _, o := range orders {
		col, err := ds.ParseIndexColumn(o)
		if err != nil {
			panic(err)
		}
		ret.SortBy = append(ret.SortBy, col)
	}
	return ret
}

var (
	prop   = ds.MkProperty
	propNI = ds.MkPropertyNI
)

func key(elems ...interface{}) *ds.Key {
	return ds.MakeKey(globalAppID, "ns", elems...)
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

// cat is a convenience method for concatenating anything with an underlying
// byte representation into a single []byte.
func cat(bytethings ...interface{}) []byte {
	err := error(nil)
	buf := &bytes.Buffer{}
	for _, thing := range bytethings {
		switch x := thing.(type) {
		case int64:
			_, err = cmpbin.WriteInt(buf, x)
		case int:
			_, err = cmpbin.WriteInt(buf, int64(x))
		case uint64:
			_, err = cmpbin.WriteUint(buf, x)
		case uint:
			_, err = cmpbin.WriteUint(buf, uint64(x))
		case float64:
			_, err = cmpbin.WriteFloat64(buf, x)
		case byte:
			err = buf.WriteByte(x)
		case ds.PropertyType:
			err = buf.WriteByte(byte(x))
		case string:
			_, err = cmpbin.WriteString(buf, x)
		case []byte:
			_, err = buf.Write(x)
		case time.Time:
			err = serialize.WriteTime(buf, x)
		case *ds.Key:
			err = serialize.WriteKey(buf, serialize.WithoutContext, x)
		case *ds.IndexDefinition:
			err = serialize.WriteIndexDefinition(buf, *x)
		case ds.Property:
			err = serialize.WriteProperty(buf, serialize.WithoutContext, x)
		default:
			panic(fmt.Errorf("I don't know how to deal with %T: %#v", thing, thing))
		}
		die(err)
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
