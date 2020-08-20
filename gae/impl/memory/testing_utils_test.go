// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/luci/common/data/cmpbin"
)

func init() {
	serializationDeterministic = true
	serialize.WritePropertyMapDeterministic = true
}

var nextMarker = "NEXT MARKER"
var Next = &nextMarker

var multiMarker = "MULTI MARKER"
var Multi = &multiMarker

// If you want an entry that is single to be treated as multi-, prepend it
// with Multi. Terminate each set of property tokens with Next.
//   pmap(
//     "prop", "val", 0, 100, Next,
//     "other", "val", 0, 100, Next,
//     "name", Multi, "value", Next,
//   )
//
//
//
func pmap(stuff ...interface{}) ds.PropertyMap {
	ret := ds.PropertyMap{}

	nom := func() (name string, toks []interface{}, multi bool) {
		var i int

		for i = 0; i < len(stuff); i++ {
			e := stuff[i]
			switch {
			case e == Next:
				stuff = stuff[i+1:]
				return
			case e == Multi:
				multi = true
			case i == 0:
				name = e.(string)
			default:
				toks = append(toks, e)
			}
		}

		stuff = nil
		return
	}

	for len(stuff) > 0 {
		pname, toks, multi := nom()

		var mp func(interface{}) ds.Property
		if pname[0] == '$' || (strings.HasPrefix(pname, "__") && strings.HasSuffix(pname, "__")) {
			mp = propNI
		} else {
			mp = prop
		}

		if len(toks) == 1 && !multi {
			ret[pname] = mp(toks[0])
		} else {
			pslice := make(ds.PropertySlice, len(toks))
			for i, tok := range toks {
				pslice[i] = mp(tok)
			}
			ret[pname] = pslice
		}
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
	return ds.MkKeyContext("dev~app", "ns").MakeKey(elems...)
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
