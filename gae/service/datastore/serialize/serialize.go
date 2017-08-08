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

package serialize

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/gae/service/blobstore"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
)

// MaxIndexColumns is the maximum number of sort columns (e.g. sort orders) that
// ReadIndexDefinition is willing to deserialize. 64 was chosen as
// a likely-astronomical number.
const MaxIndexColumns = 64

// WritePropertyMapDeterministic allows tests to make WritePropertyMap
// deterministic.
var WritePropertyMapDeterministic = false

// ReadPropertyMapReasonableLimit sets a limit on the number of rows and
// number of properties per row which can be read by ReadPropertyMap. The
// total number of Property objects readable by this method is this number
// squared (e.g. Limit rows * Limit properties)
const ReadPropertyMapReasonableLimit uint64 = 30000

// ReadKeyNumToksReasonableLimit is the maximum number of Key tokens that
// ReadKey is willing to read for a single key.
const ReadKeyNumToksReasonableLimit = 50

// KeyContext controls whether the various Write and Read serializtion
// routines should encode the context of Keys (read: the appid and namespace).
// Frequently the appid and namespace of keys are known in advance and so there's
// no reason to redundantly encode them.
type KeyContext bool

// With- and WithoutContext indicate if the serialization method should include
// context for Keys. See KeyContext for more information.
const (
	WithContext    KeyContext = true
	WithoutContext            = false
)

// WriteKey encodes a key to the buffer. If context is WithContext, then this
// encoded value will include the appid and namespace of the key.
func WriteKey(buf WriteBuffer, context KeyContext, k *ds.Key) (err error) {
	// [appid ++ namespace]? ++ [1 ++ token]* ++ NULL
	defer recoverTo(&err)
	appid, namespace, toks := k.Split()
	if context == WithContext {
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
		panicIf(WriteKeyTok(buf, tok))
	}
	return buf.WriteByte(0)
}

// ReadKey deserializes a key from the buffer. The value of context must match
// the value of context that was passed to WriteKey when the key was encoded.
// If context == WithoutContext, then the appid and namespace parameters are
// used in the decoded Key. Otherwise they're ignored.
func ReadKey(buf ReadBuffer, context KeyContext, inKC ds.KeyContext) (ret *ds.Key, err error) {
	defer recoverTo(&err)
	actualCtx, e := buf.ReadByte()
	panicIf(e)

	var kc ds.KeyContext
	if actualCtx == 1 {
		kc.AppID, _, e = cmpbin.ReadString(buf)
		panicIf(e)
		kc.Namespace, _, e = cmpbin.ReadString(buf)
		panicIf(e)
	} else if actualCtx != 0 {
		err = fmt.Errorf("helper: expected actualCtx to be 0 or 1, got %d", actualCtx)
		return
	}

	if context == WithoutContext {
		// overrwrite with the supplied ones
		kc = inKC
	}

	toks := []ds.KeyTok{}
	for {
		ctrlByte, e := buf.ReadByte()
		panicIf(e)
		if ctrlByte == 0 {
			break
		}
		if len(toks)+1 > ReadKeyNumToksReasonableLimit {
			err = fmt.Errorf(
				"helper: tried to decode huge key with > %d tokens",
				ReadKeyNumToksReasonableLimit)
			return
		}

		tok, e := ReadKeyTok(buf)
		panicIf(e)

		toks = append(toks, tok)
	}

	return kc.NewKeyToks(toks), nil
}

// WriteKeyTok writes a KeyTok to the buffer. You usually want WriteKey
// instead of this.
func WriteKeyTok(buf WriteBuffer, tok ds.KeyTok) (err error) {
	// tok.kind ++ typ ++ [tok.stringID || tok.intID]
	defer recoverTo(&err)
	_, e := cmpbin.WriteString(buf, tok.Kind)
	panicIf(e)
	if tok.StringID != "" {
		panicIf(buf.WriteByte(byte(ds.PTString)))
		_, e := cmpbin.WriteString(buf, tok.StringID)
		panicIf(e)
	} else {
		panicIf(buf.WriteByte(byte(ds.PTInt)))
		_, e := cmpbin.WriteInt(buf, tok.IntID)
		panicIf(e)
	}
	return nil
}

// ReadKeyTok reads a KeyTok from the buffer. You usually want ReadKey
// instead of this.
func ReadKeyTok(buf ReadBuffer) (ret ds.KeyTok, err error) {
	defer recoverTo(&err)
	e := error(nil)
	ret.Kind, _, e = cmpbin.ReadString(buf)
	panicIf(e)

	typ, e := buf.ReadByte()
	panicIf(e)

	switch ds.PropertyType(typ) {
	case ds.PTString:
		ret.StringID, _, err = cmpbin.ReadString(buf)
	case ds.PTInt:
		ret.IntID, _, err = cmpbin.ReadInt(buf)
		if err == nil && ret.IntID <= 0 {
			err = errors.New("helper: decoded key with empty stringID and zero/negative intID")
		}
	default:
		err = fmt.Errorf("helper: invalid type %s", ds.PropertyType(typ))
	}
	return
}

// WriteGeoPoint writes a GeoPoint to the buffer.
func WriteGeoPoint(buf WriteBuffer, gp ds.GeoPoint) (err error) {
	defer recoverTo(&err)
	_, e := cmpbin.WriteFloat64(buf, gp.Lat)
	panicIf(e)
	_, e = cmpbin.WriteFloat64(buf, gp.Lng)
	return e
}

// ReadGeoPoint reads a GeoPoint from the buffer.
func ReadGeoPoint(buf ReadBuffer) (gp ds.GeoPoint, err error) {
	defer recoverTo(&err)
	e := error(nil)
	gp.Lat, _, e = cmpbin.ReadFloat64(buf)
	panicIf(e)

	gp.Lng, _, e = cmpbin.ReadFloat64(buf)
	panicIf(e)

	if !gp.Valid() {
		err = fmt.Errorf("helper: decoded invalid GeoPoint: %v", gp)
	}
	return
}

// WriteTime writes a time.Time to the buffer.
//
// The supplied time is rounded via datastore.RoundTime and written as a
// microseconds-since-epoch integer to comform to datastore storage standards.
func WriteTime(buf WriteBuffer, t time.Time) error {
	name, off := t.Zone()
	if name != "UTC" || off != 0 {
		panic(fmt.Errorf("helper: UTC OR DEATH: %s", t))
	}

	_, err := cmpbin.WriteInt(buf, ds.TimeToInt(t))
	return err
}

// ReadTime reads a time.Time from the buffer.
func ReadTime(buf ReadBuffer) (time.Time, error) {
	v, _, err := cmpbin.ReadInt(buf)
	if err != nil {
		return time.Time{}, err
	}
	return ds.IntToTime(v), nil
}

// WriteProperty writes a Property to the buffer. `context` behaves the same
// way that it does for WriteKey, but only has an effect if `p` contains a
// Key as its IndexValue.
func WriteProperty(buf WriteBuffer, context KeyContext, p ds.Property) error {
	return writePropertyImpl(buf, context, &p, false)
}

// WriteIndexProperty writes a Property to the buffer as its native index type.
// `context` behaves the same way that it does for WriteKey, but only has an
// effect if `p` contains a Key as its IndexValue.
func WriteIndexProperty(buf WriteBuffer, context KeyContext, p ds.Property) error {
	return writePropertyImpl(buf, context, &p, true)
}

// writePropertyImpl is an implementation of WriteProperty and
// WriteIndexProperty.
func writePropertyImpl(buf WriteBuffer, context KeyContext, p *ds.Property, index bool) (err error) {
	defer recoverTo(&err)

	it, v := p.IndexTypeAndValue()
	if !index {
		it = p.Type()
	}
	typb := byte(it)
	if p.IndexSetting() != ds.NoIndex {
		typb |= 0x80
	}
	panicIf(buf.WriteByte(typb))

	err = writeIndexValue(buf, context, v)
	return
}

// writeIndexValue writes the index value of v to buf.
//
// v may be one of the return types from ds.Property's GetIndexTypeAndValue
// method.
func writeIndexValue(buf WriteBuffer, context KeyContext, v interface{}) (err error) {
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
	case ds.GeoPoint:
		err = WriteGeoPoint(buf, t)
	case *ds.Key:
		err = WriteKey(buf, context, t)

	default:
		err = fmt.Errorf("unsupported type: %T", t)
	}
	return
}

// ReadProperty reads a Property from the buffer. `context` and `kc` behave the
// same way they do for ReadKey, but only have an effect if the decoded property
// has a Key value.
func ReadProperty(buf ReadBuffer, context KeyContext, kc ds.KeyContext) (p ds.Property, err error) {
	val := interface{}(nil)
	b, err := buf.ReadByte()
	if err != nil {
		return
	}
	is := ds.ShouldIndex
	if (b & 0x80) == 0 {
		is = ds.NoIndex
	}
	switch ds.PropertyType(b & 0x7f) {
	case ds.PTNull:
	case ds.PTBool:
		b, err = buf.ReadByte()
		val = (b != 0)
	case ds.PTInt:
		val, _, err = cmpbin.ReadInt(buf)
	case ds.PTFloat:
		val, _, err = cmpbin.ReadFloat64(buf)
	case ds.PTString:
		val, _, err = cmpbin.ReadString(buf)
	case ds.PTBytes:
		val, _, err = cmpbin.ReadBytes(buf)
	case ds.PTTime:
		val, err = ReadTime(buf)
	case ds.PTGeoPoint:
		val, err = ReadGeoPoint(buf)
	case ds.PTKey:
		val, err = ReadKey(buf, context, kc)
	case ds.PTBlobKey:
		s := ""
		if s, _, err = cmpbin.ReadString(buf); err != nil {
			break
		}
		val = blobstore.Key(s)
	default:
		err = fmt.Errorf("read: unknown type! %v", b)
	}
	if err == nil {
		err = p.SetValue(val, is)
	}
	return
}

// WritePropertyMap writes an entire PropertyMap to the buffer. `context`
// behaves the same way that it does for WriteKey.
//
// If WritePropertyMapDeterministic is true, then the rows will be sorted by
// property name before they're serialized to buf (mostly useful for testing,
// but also potentially useful if you need to make a hash of the property data).
//
// Write skips metadata keys.
func WritePropertyMap(buf WriteBuffer, context KeyContext, pm ds.PropertyMap) (err error) {
	defer recoverTo(&err)
	rows := make(sort.StringSlice, 0, len(pm))
	tmpBuf := &bytes.Buffer{}
	pm, _ = pm.Save(false)
	for name, pdata := range pm {
		tmpBuf.Reset()
		_, e := cmpbin.WriteString(tmpBuf, name)
		panicIf(e)

		switch t := pdata.(type) {
		case ds.Property:
			_, e = cmpbin.WriteInt(tmpBuf, -1)
			panicIf(e)
			panicIf(WriteProperty(tmpBuf, context, t))

		case ds.PropertySlice:
			_, e = cmpbin.WriteInt(tmpBuf, int64(len(t)))
			panicIf(e)
			for _, p := range t {
				panicIf(WriteProperty(tmpBuf, context, p))
			}

		default:
			return fmt.Errorf("unknown PropertyData type %T", t)
		}
		rows = append(rows, tmpBuf.String())
	}

	if WritePropertyMapDeterministic {
		rows.Sort()
	}

	_, e := cmpbin.WriteUint(buf, uint64(len(pm)))
	panicIf(e)
	for _, r := range rows {
		_, e := buf.WriteString(r)
		panicIf(e)
	}
	return
}

// ReadPropertyMap reads a PropertyMap from the buffer. `context` and
// friends behave the same way that they do for ReadKey.
func ReadPropertyMap(buf ReadBuffer, context KeyContext, kc ds.KeyContext) (pm ds.PropertyMap, err error) {
	defer recoverTo(&err)

	numRows := uint64(0)
	numRows, _, e := cmpbin.ReadUint(buf)
	panicIf(e)
	if numRows > ReadPropertyMapReasonableLimit {
		err = fmt.Errorf("helper: tried to decode map with huge number of rows %d", numRows)
		return
	}

	pm = make(ds.PropertyMap, numRows)

	name, prop := "", ds.Property{}
	for i := uint64(0); i < numRows; i++ {
		name, _, e = cmpbin.ReadString(buf)
		panicIf(e)

		numProps, _, e := cmpbin.ReadInt(buf)
		panicIf(e)
		switch {
		case numProps < 0:
			// Single property.
			prop, err = ReadProperty(buf, context, kc)
			panicIf(err)
			pm[name] = prop

		case uint64(numProps) > ReadPropertyMapReasonableLimit:
			err = fmt.Errorf("helper: tried to decode map with huge number of properties %d", numProps)
			return

		default:
			props := make(ds.PropertySlice, 0, numProps)
			for j := int64(0); j < numProps; j++ {
				prop, err = ReadProperty(buf, context, kc)
				panicIf(err)
				props = append(props, prop)
			}
			pm[name] = props
		}
	}
	return
}

// WriteIndexColumn writes an IndexColumn to the buffer.
func WriteIndexColumn(buf WriteBuffer, c ds.IndexColumn) (err error) {
	defer recoverTo(&err)

	if !c.Descending {
		panicIf(buf.WriteByte(0))
	} else {
		panicIf(buf.WriteByte(1))
	}
	_, err = cmpbin.WriteString(buf, c.Property)
	return
}

// ReadIndexColumn reads an IndexColumn from the buffer.
func ReadIndexColumn(buf ReadBuffer) (c ds.IndexColumn, err error) {
	defer recoverTo(&err)

	dir, err := buf.ReadByte()
	panicIf(err)

	c.Descending = dir != 0
	c.Property, _, err = cmpbin.ReadString(buf)
	return
}

// WriteIndexDefinition writes an IndexDefinition to the buffer
func WriteIndexDefinition(buf WriteBuffer, i ds.IndexDefinition) (err error) {
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
		panicIf(WriteIndexColumn(buf, sb))
	}
	return buf.WriteByte(0)
}

// ReadIndexDefinition reads an IndexDefinition from the buffer.
func ReadIndexDefinition(buf ReadBuffer) (i ds.IndexDefinition, err error) {
	defer recoverTo(&err)

	i.Kind, _, err = cmpbin.ReadString(buf)
	panicIf(err)

	anc, err := buf.ReadByte()
	panicIf(err)

	i.Ancestor = anc == 1

	for {
		ctrl := byte(0)
		ctrl, err = buf.ReadByte()
		panicIf(err)
		if ctrl == 0 {
			break
		}
		if len(i.SortBy) > MaxIndexColumns {
			err = fmt.Errorf("datastore: Got over %d sort orders", MaxIndexColumns)
			return
		}

		sb, err := ReadIndexColumn(buf)
		panicIf(err)

		i.SortBy = append(i.SortBy, sb)
	}

	return
}

// SerializedPslice is all of the serialized DSProperty values in ASC order.
type SerializedPslice [][]byte

func (s SerializedPslice) Len() int           { return len(s) }
func (s SerializedPslice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s SerializedPslice) Less(i, j int) bool { return bytes.Compare(s[i], s[j]) < 0 }

// PropertySlice serializes a single row of a DSProperty map.
//
// It does not differentiate between single- and multi- properties.
func PropertySlice(vals ds.PropertySlice) SerializedPslice {
	dups := stringset.New(0)
	ret := make(SerializedPslice, 0, len(vals))
	for _, v := range vals {
		if v.IndexSetting() == ds.NoIndex {
			continue
		}

		data := ToBytes(v)
		dataS := string(data)
		if !dups.Add(dataS) {
			continue
		}
		ret = append(ret, data)
	}
	return ret
}

// SerializedPmap maps from
//   prop name -> [<serialized DSProperty>, ...]
// includes special values '__key__' and '__ancestor__' which contains all of
// the ancestor entries for this key.
type SerializedPmap map[string]SerializedPslice

// PropertyMapPartially turns a regular PropertyMap into a SerializedPmap.
// Essentially all the []Property's become SerializedPslice, using cmpbin and
// datastore/serialize's encodings.
func PropertyMapPartially(k *ds.Key, pm ds.PropertyMap) (ret SerializedPmap) {
	ret = make(SerializedPmap, len(pm)+2)
	if k != nil {
		ret["__key__"] = [][]byte{ToBytes(ds.MkProperty(k))}
		for k != nil {
			ret["__ancestor__"] = append(ret["__ancestor__"], ToBytes(ds.MkProperty(k)))
			k = k.Parent()
		}
	}
	for k := range pm {
		newVals := PropertySlice(pm.Slice(k))
		if len(newVals) > 0 {
			ret[k] = newVals
		}
	}
	return
}

func toBytesErr(i interface{}, ctx KeyContext) (ret []byte, err error) {
	buf := bytes.Buffer{}

	switch t := i.(type) {
	case ds.IndexColumn:
		err = WriteIndexColumn(&buf, t)

	case ds.IndexDefinition:
		err = WriteIndexDefinition(&buf, t)

	case ds.KeyTok:
		err = WriteKeyTok(&buf, t)

	case ds.Property:
		err = WriteIndexProperty(&buf, ctx, t)

	case ds.PropertyMap:
		err = WritePropertyMap(&buf, ctx, t)

	default:
		_, v := ds.MkProperty(i).IndexTypeAndValue()
		err = writeIndexValue(&buf, ctx, v)
	}

	if err == nil {
		ret = buf.Bytes()
	}
	return
}

// ToBytesErr serializes i to a byte slice, if it's one of the type supported
// by this library, otherwise it returns an error.
//
// Key types will be serialized using the 'WithoutContext' option (e.g. their
// encoded forms will not contain AppID or Namespace).
func ToBytesErr(i interface{}) ([]byte, error) {
	return toBytesErr(i, WithoutContext)
}

// ToBytesWithContextErr serializes i to a byte slice, if it's one of the type
// supported by this library, otherwise it returns an error.
//
// Key types will be serialized using the 'WithContext' option (e.g. their
// encoded forms will contain AppID and Namespace).
func ToBytesWithContextErr(i interface{}) ([]byte, error) {
	return toBytesErr(i, WithContext)
}

// ToBytes serializes i to a byte slice, if it's one of the type supported
// by this library. If an error is encountered (e.g. `i` is not a supported
// type), this method panics.
//
// Key types will be serialized using the 'WithoutContext' option (e.g. their
// encoded forms will not contain AppID or Namespace).
func ToBytes(i interface{}) []byte {
	ret, err := ToBytesErr(i)
	if err != nil {
		panic(err)
	}
	return ret
}

// ToBytesWithContext serializes i to a byte slice, if it's one of the type
// supported by this library. If an error is encountered (e.g. `i` is not
// a supported type), this method panics.
//
// Key types will be serialized using the 'WithContext' option (e.g. their
// encoded forms will not contain AppID or Namespace).
func ToBytesWithContext(i interface{}) []byte {
	ret, err := ToBytesWithContextErr(i)
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
