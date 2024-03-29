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

package datastore

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
)

// WritePropertyMapDeterministic allows tests to make Serializer.PropertyMap
// deterministic.
//
// This should be set once at the top of your tests in an init() function.
var WritePropertyMapDeterministic = false

// Serializer allows writing binary-encoded datastore types (like Properties,
// Keys, etc.)
//
// See the `Serialize` and `SerializeKC` variables for common shortcuts.
type Serializer struct {
	// If true, WithKeyContext controls whether bytes written with this Serializer
	// include the Key's appid and namespace.
	//
	// Frequently the appid and namespace of keys are known in advance and so
	// there's no reason to redundantly encode them.
	WithKeyContext bool
}

var (
	// Serialize is a Serializer{WithKeyContext:false}, useful for inline
	// invocations like:
	//
	//   datastore.Serialize.Time(...)
	Serialize Serializer

	// SerializeKC is a Serializer{WithKeyContext:true}, useful for inline
	// invocations like:
	//
	//   datastore.SerializeKC.Key(...)
	SerializeKC = Serializer{true}
)

// Key encodes a key to the buffer. If context is WithContext, then this
// encoded value will include the appid and namespace of the key.
func (s Serializer) Key(buf cmpbin.WriteableBytesBuffer, k *Key) (err error) {
	// [appid ++ namespace]? ++ [1 ++ token]* ++ NULL
	defer recoverTo(&err)
	appid, namespace, toks := k.Split()
	if s.WithKeyContext {
		panicIf(buf.WriteByte(1))
		_, e := cmpbin.WriteString(buf, appid)
		panicIf(e)
		_, e = cmpbin.WriteString(buf, namespace)
		panicIf(e)
	} else {
		panicIf(buf.WriteByte(0))
	}
	for _, tok := range toks {
		panicIf(buf.WriteByte(1))
		panicIf(s.KeyTok(buf, tok))
	}
	return buf.WriteByte(0)
}

// KeyTok writes a KeyTok to the buffer. You usually want Key instead of this.
func (s Serializer) KeyTok(buf cmpbin.WriteableBytesBuffer, tok KeyTok) (err error) {
	// tok.kind ++ typ ++ [tok.stringID || tok.intID]
	defer recoverTo(&err)
	_, e := cmpbin.WriteString(buf, tok.Kind)
	panicIf(e)
	if tok.StringID != "" {
		panicIf(buf.WriteByte(byte(PTString)))
		_, e := cmpbin.WriteString(buf, tok.StringID)
		panicIf(e)
	} else {
		panicIf(buf.WriteByte(byte(PTInt)))
		_, e := cmpbin.WriteInt(buf, tok.IntID)
		panicIf(e)
	}
	return nil
}

// GeoPoint writes a GeoPoint to the buffer.
func (s Serializer) GeoPoint(buf cmpbin.WriteableBytesBuffer, gp GeoPoint) (err error) {
	defer recoverTo(&err)
	_, e := cmpbin.WriteFloat64(buf, gp.Lat)
	panicIf(e)
	_, e = cmpbin.WriteFloat64(buf, gp.Lng)
	return e
}

// Time writes a time.Time to the buffer.
//
// The supplied time is rounded via datastore.RoundTime and written as a
// microseconds-since-epoch integer to comform to datastore storage standards.
func (s Serializer) Time(buf cmpbin.WriteableBytesBuffer, t time.Time) error {
	name, off := t.Zone()
	if name != "UTC" || off != 0 {
		panic(fmt.Errorf("helper: UTC OR DEATH: %s", t))
	}

	_, err := cmpbin.WriteInt(buf, TimeToInt(t))
	return err
}

// Property writes a Property to the buffer. `context` behaves the same
// way that it does for WriteKey, but only has an effect if `p` contains a
// Key as its IndexValue.
func (s Serializer) Property(buf cmpbin.WriteableBytesBuffer, p Property) error {
	return s.propertyImpl(buf, &p, false)
}

// IndexedProperty writes a Property to the buffer as its native index type.
// `context` behaves the same way that it does for WriteKey, but only has an
// effect if `p` contains a Key as its IndexValue.
func (s Serializer) IndexedProperty(buf cmpbin.WriteableBytesBuffer, p Property) error {
	return s.propertyImpl(buf, &p, true)
}

// propertyImpl is an implementation of Property and IndexedProperty.
func (s Serializer) propertyImpl(buf cmpbin.WriteableBytesBuffer, p *Property, index bool) (err error) {
	defer recoverTo(&err)

	var it PropertyType
	var v any

	if !p.Type().Comparable() {
		// Non-comparable types are stored as is and can't be used in indexes.
		it, v = p.Type(), p.Value()
		if index {
			return fmt.Errorf("serializing uncomparable type %s as indexed property", p.Type())
		}
	} else {
		// For comparable indexable types do type coercion to an indexed value. Note
		// that this means that e.g. PTTime is *always* stored as int64. If `index`
		// is true (meaning we are going to compare the final byte blob to other
		// byte blobs in the index), the property is also tagged by PTInt type
		// (instead of its native PTTime). That way all properties represented by
		// e.g. int64 are comparable to one another as raw byte blobs, regardless of
		// their "public" type.
		it, v = p.IndexTypeAndValue()
		if !index {
			it = p.Type()
		}
	}

	// Note: non-comparable properties still can have "indexed" flag set. It is
	// just interpreted differently (in PTPropertyMap case it means to index
	// the inner properties).
	typb := byte(it)
	if p.IndexSetting() != NoIndex {
		typb |= 0x80
	}
	panicIf(buf.WriteByte(typb))
	err = s.rawPropValue(buf, v)
	return
}

// rawPropValue writes the value of v to buf.
//
// `v` is either a type produced by IndexTypeAndValue() or some uncomparable
// type (e.g. PropertyMap).
func (s Serializer) rawPropValue(buf cmpbin.WriteableBytesBuffer, v any) (err error) {
	switch t := v.(type) {
	case nil:
	case bool:
		b := byte(0)
		if t {
			b = 1
		}
		err = buf.WriteByte(b)
	case int64:
		_, err = cmpbin.WriteInt(buf, t)
	case float64:
		_, err = cmpbin.WriteFloat64(buf, t)
	case string:
		_, err = cmpbin.WriteString(buf, t)
	case []byte:
		_, err = cmpbin.WriteBytes(buf, t)
	case GeoPoint:
		err = s.GeoPoint(buf, t)
	case PropertyMap:
		// This is a property map representing a nested entity. Cloud Datastore can
		// store their keys, but only if they are non-partial. Do the same here. If
		// `t` has `$key`, its key context will be used, otherwise there will be
		// no key context at all (it will be populated when deserializing).
		key, _ := (KeyContext{}).NewKeyFromMeta(t)
		if key != nil && key.IsIncomplete() {
			key = nil
		}
		err = s.propertyMap(buf, t, key)
	case *Key:
		err = s.Key(buf, t)

	default:
		err = fmt.Errorf("unsupported type: %T", t)
	}
	return
}

// PropertyMap writes an entire PropertyMap to the buffer. `context`
// behaves the same way that it does for WriteKey.
//
// If WritePropertyMapDeterministic is true, then the rows will be sorted by
// property name before they're serialized to buf (mostly useful for testing,
// but also potentially useful if you need to make a hash of the property data).
func (s Serializer) PropertyMap(buf cmpbin.WriteableBytesBuffer, pm PropertyMap) error {
	return s.propertyMap(buf, pm, nil)
}

// propertyMap writes an entire PropertyMap to the buffer.
//
// If `key` is given, it will be serialized as a `$key` meta property. All other
// meta properties are skipped.
func (s Serializer) propertyMap(buf cmpbin.WriteableBytesBuffer, pm PropertyMap, key *Key) (err error) {
	defer recoverTo(&err)
	rows := make(sort.StringSlice, 0, len(pm)+1)
	tmpBuf := &bytes.Buffer{}

	writeProp := func(name string, pdata PropertyData) {
		tmpBuf.Reset()
		_, e := cmpbin.WriteString(tmpBuf, name)
		panicIf(e)

		switch t := pdata.(type) {
		case Property:
			_, e = cmpbin.WriteInt(tmpBuf, -1)
			panicIf(e)
			panicIf(s.Property(tmpBuf, t))

		case PropertySlice:
			_, e = cmpbin.WriteInt(tmpBuf, int64(len(t)))
			panicIf(e)
			for _, p := range t {
				panicIf(s.Property(tmpBuf, p))
			}

		default:
			panicIf(fmt.Errorf("unknown PropertyData type %T", t))
		}

		rows = append(rows, tmpBuf.String())
	}

	if key != nil {
		writeProp("$key", MkPropertyNI(key))
	}

	for name, pdata := range pm {
		if !isMetaKey(name) {
			writeProp(name, pdata)
		}
	}

	if WritePropertyMapDeterministic {
		rows.Sort()
	}

	_, e := cmpbin.WriteUint(buf, uint64(len(rows)))
	panicIf(e)
	for _, r := range rows {
		_, e := buf.WriteString(r)
		panicIf(e)
	}
	return
}

// IndexColumn writes an IndexColumn to the buffer.
func (s Serializer) IndexColumn(buf cmpbin.WriteableBytesBuffer, c IndexColumn) (err error) {
	defer recoverTo(&err)

	if !c.Descending {
		panicIf(buf.WriteByte(0))
	} else {
		panicIf(buf.WriteByte(1))
	}
	_, err = cmpbin.WriteString(buf, c.Property)
	return
}

// IndexDefinition writes an IndexDefinition to the buffer
func (s Serializer) IndexDefinition(buf cmpbin.WriteableBytesBuffer, i IndexDefinition) (err error) {
	defer recoverTo(&err)

	_, err = cmpbin.WriteString(buf, i.Kind)
	panicIf(err)
	if !i.Ancestor {
		panicIf(buf.WriteByte(0))
	} else {
		panicIf(buf.WriteByte(1))
	}
	for _, sb := range i.SortBy {
		panicIf(buf.WriteByte(1))
		panicIf(s.IndexColumn(buf, sb))
	}
	return buf.WriteByte(0)
}

// IndexedPropertySlice is a set of properties serialized to their comparable
// index representations via Serializer.IndexedProperties(...).
//
// Values are in some arbitrary order. If you need them sorted, call sort.Sort
// explicitly.
type IndexedPropertySlice [][]byte

func (s IndexedPropertySlice) Len() int           { return len(s) }
func (s IndexedPropertySlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s IndexedPropertySlice) Less(i, j int) bool { return bytes.Compare(s[i], s[j]) < 0 }

// IndexedProperties maps from a property name to a set of its indexed values.
//
// It includes special values '__key__' and '__ancestor__' which contains all of
// the ancestor entries for this key.
//
// Map values are in some arbitrary order. If you need them sorted, call Sort
// explicitly.
type IndexedProperties map[string]IndexedPropertySlice

// Sort sorts all values (useful in tests).
func (sip IndexedProperties) Sort() {
	for _, v := range sip {
		sort.Sort(v)
	}
}

// indexedPropsBuilder is used to construct IndexedProperties.
type indexedPropsBuilder map[string]stringset.Set

// add adds an entry to the set under the given key.
func (b indexedPropsBuilder) add(key string, val []byte) {
	if vals := b[key]; vals != nil {
		vals.Add(string(val))
	} else {
		b[key] = stringset.Set{string(val): struct{}{}}
	}
}

// visit recursively traverses a property map, adding all indexed properties.
func (b indexedPropsBuilder) visit(propNamePfx string, pm PropertyMap) {
	for k := range pm {
		if isMetaKey(k) {
			continue
		}
		for _, v := range pm.Slice(k) {
			if v.IndexSetting() == NoIndex {
				continue
			}
			switch {
			case v.Type() == PTPropertyMap:
				// Nested property maps are indexed recursively per-field, together with
				// a special `__key__` property representing the embedded key, as long
				// as it is a complete key.
				nested := v.Value().(PropertyMap)
				if key, _ := (KeyContext{}).NewKeyFromMeta(nested); key != nil && !key.IsIncomplete() {
					b.add(propNamePfx+k+".__key__", Serialize.ToBytes(MkProperty(key)))
				}
				b.visit(propNamePfx+k+".", nested)
			case v.Type().Comparable():
				// Regular indexed properties that can be converted to byte blobs.
				b.add(propNamePfx+k, Serialize.ToBytes(v))
			default:
				panic(fmt.Sprintf("uncomparable type %q being indexed as %q", v.Type(), propNamePfx+k))
			}
		}
	}
}

// collect converts all added properties into the final IndexedProperties map.
func (b indexedPropsBuilder) collect() IndexedProperties {
	res := make(IndexedProperties, len(b))
	for k, v := range b {
		blobs := make([][]byte, 0, v.Len())
		for str := range v {
			blobs = append(blobs, []byte(str))
		}
		res[k] = blobs
	}
	return res
}

// IndexedProperties turns a regular PropertyMap into a IndexedProperties.
//
// Essentially all the []Property's become IndexedPropertySlice, using cmpbin
// and Serializer's encodings.
//
// Keys are serialized without their context.
func (s Serializer) IndexedProperties(k *Key, pm PropertyMap) IndexedProperties {
	builder := make(indexedPropsBuilder, len(pm)+2)
	if k != nil {
		builder.add("__key__", Serialize.ToBytes(MkProperty(k)))
		for k != nil {
			builder.add("__ancestor__", Serialize.ToBytes(MkProperty(k)))
			k = k.Parent()
		}
	}
	builder.visit("", pm)
	return builder.collect()
}

// IndexedPropertiesForIndicies is like IndexedProperties, but it returns only
// properties mentioned in the given indices as well as special '__key__' and
// '__ancestor__' properties.
//
// It is just an optimization to avoid serializing indices that will never be
// checked.
func (s Serializer) IndexedPropertiesForIndicies(k *Key, pm PropertyMap, idx []IndexColumn) (ret IndexedProperties) {
	// TODO(vadimsh): Really make it an optimization. This function is only used
	// in txnbuf, which is currently essentially dead code.

	res := make(IndexedProperties, len(idx)+2)
	sip := s.IndexedProperties(k, pm)

	move := func(key string) {
		if v := sip[key]; len(v) > 0 {
			res[key] = v
		}
	}

	move("__key__")
	move("__ancestor__")
	for _, col := range idx {
		move(col.Property)
	}

	return res
}

// ToBytesErr serializes i to a byte slice, if it's one of the type supported
// by this library, otherwise it returns an error.
func (s Serializer) ToBytesErr(i any) (ret []byte, err error) {
	buf := bytes.Buffer{}

	switch t := i.(type) {
	case IndexColumn:
		err = s.IndexColumn(&buf, t)

	case IndexDefinition:
		err = s.IndexDefinition(&buf, t)

	case KeyTok:
		err = s.KeyTok(&buf, t)

	case Property:
		err = s.IndexedProperty(&buf, t)

	case PropertyMap:
		err = s.PropertyMap(&buf, t)

	default:
		// Do the same as Property(...), except do not write the type tag.
		prop := MkProperty(i)
		if prop.Type().Comparable() {
			_, v := prop.IndexTypeAndValue()
			err = s.rawPropValue(&buf, v)
		} else {
			err = s.rawPropValue(&buf, prop.Value())
		}
	}

	if err == nil {
		ret = buf.Bytes()
	}
	return
}

// ToBytes serializes i to a byte slice, if it's one of the type supported
// by this library. If an error is encountered (e.g. `i` is not a supported
// type), this method panics.
func (s Serializer) ToBytes(i any) []byte {
	ret, err := s.ToBytesErr(i)
	if err != nil {
		panic(err)
	}
	return ret
}

type parseError error

func panicIf(err error) {
	if err != nil {
		panic(parseError(err))
	}
}

func recoverTo(err *error) {
	if r := recover(); r != nil {
		if rerr := r.(parseError); rerr != nil {
			*err = error(rerr)
		}
	}
}
