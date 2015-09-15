// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serialize

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/luci/gae/service/blobstore"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"github.com/luci/luci-go/common/cmpbin"
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
func WriteKey(buf Buffer, context KeyContext, k ds.Key) (err error) {
	// [appid ++ namespace]? ++ [1 ++ token]* ++ NULL
	defer recoverTo(&err)
	appid, namespace, toks := dskey.Split(k)
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
func ReadKey(buf Buffer, context KeyContext, appid, namespace string) (ret ds.Key, err error) {
	defer recoverTo(&err)
	actualCtx, e := buf.ReadByte()
	panicIf(e)

	actualAid, actualNS := "", ""
	if actualCtx == 1 {
		actualAid, _, e = cmpbin.ReadString(buf)
		panicIf(e)
		actualNS, _, e = cmpbin.ReadString(buf)
		panicIf(e)
	} else if actualCtx != 0 {
		err = fmt.Errorf("helper: expected actualCtx to be 0 or 1, got %d", actualCtx)
		return
	}

	if context == WithoutContext {
		// overrwrite with the supplied ones
		actualAid = appid
		actualNS = namespace
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

	return dskey.NewToks(actualAid, actualNS, toks), nil
}

// WriteKeyTok writes a KeyTok to the buffer. You usually want WriteKey
// instead of this.
func WriteKeyTok(buf Buffer, tok ds.KeyTok) (err error) {
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
func ReadKeyTok(buf Buffer) (ret ds.KeyTok, err error) {
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
func WriteGeoPoint(buf Buffer, gp ds.GeoPoint) (err error) {
	defer recoverTo(&err)
	_, e := cmpbin.WriteFloat64(buf, gp.Lat)
	panicIf(e)
	_, e = cmpbin.WriteFloat64(buf, gp.Lng)
	return e
}

// ReadGeoPoint reads a GeoPoint from the buffer.
func ReadGeoPoint(buf Buffer) (gp ds.GeoPoint, err error) {
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

// WriteTime writes a time.Time in a byte-sortable way.
//
// This method truncates the time to microseconds and drops the timezone,
// because that's the (undocumented) way that the appengine SDK does it.
func WriteTime(buf Buffer, t time.Time) error {
	name, off := t.Zone()
	if name != "UTC" || off != 0 {
		panic(fmt.Errorf("helper: UTC OR DEATH: %s", t))
	}
	_, err := cmpbin.WriteInt(buf, t.Unix()*1e6+int64(t.Nanosecond()/1e3))
	return err
}

// ReadTime reads a time.Time from the buffer.
func ReadTime(buf Buffer) (time.Time, error) {
	v, _, err := cmpbin.ReadInt(buf)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(v/1e6, (v%1e6)*1e3).UTC(), nil
}

// WriteProperty writes a Property to the buffer. `context` behaves the same
// way that it does for WriteKey, but only has an effect if `p` contains a
// Key as its Value.
func WriteProperty(buf Buffer, context KeyContext, p ds.Property) (err error) {
	defer recoverTo(&err)
	typb := byte(p.Type())
	if p.IndexSetting() != ds.NoIndex {
		typb |= 0x80
	}
	panicIf(buf.WriteByte(typb))
	switch p.Type() {
	case ds.PTNull:
	case ds.PTBool:
		b := p.Value().(bool)
		if b {
			err = buf.WriteByte(1)
		} else {
			err = buf.WriteByte(0)
		}
	case ds.PTInt:
		_, err = cmpbin.WriteInt(buf, p.Value().(int64))
	case ds.PTFloat:
		_, err = cmpbin.WriteFloat64(buf, p.Value().(float64))
	case ds.PTString:
		_, err = cmpbin.WriteString(buf, p.Value().(string))
	case ds.PTBytes:
		_, err = cmpbin.WriteBytes(buf, p.Value().([]byte))
	case ds.PTTime:
		err = WriteTime(buf, p.Value().(time.Time))
	case ds.PTGeoPoint:
		err = WriteGeoPoint(buf, p.Value().(ds.GeoPoint))
	case ds.PTKey:
		err = WriteKey(buf, context, p.Value().(ds.Key))
	case ds.PTBlobKey:
		_, err = cmpbin.WriteString(buf, string(p.Value().(blobstore.Key)))
	}
	return
}

// ReadProperty reads a Property from the buffer. `context`, `appid`, and
// `namespace` behave the same way they do for ReadKey, but only have an
// effect if the decoded property has a Key value.
func ReadProperty(buf Buffer, context KeyContext, appid, namespace string) (p ds.Property, err error) {
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
		val, err = ReadKey(buf, context, appid, namespace)
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

// WritePropertyMap writes an entire PropertyMap to the buffer. `context` behaves the same
// way that it does for WriteKey. If WritePropertyMapDeterministic is true, then
// the rows will be sorted by property name before they're serialized to buf
// (mostly useful for testing, but also potentially useful if you need to make
// a hash of the property data).
//
// Write skips metadata keys.
func WritePropertyMap(buf Buffer, context KeyContext, pm ds.PropertyMap) (err error) {
	defer recoverTo(&err)
	rows := make(sort.StringSlice, 0, len(pm))
	tmpBuf := &bytes.Buffer{}
	pm, _ = pm.Save(false)
	for name, vals := range pm {
		tmpBuf.Reset()
		_, e := cmpbin.WriteString(tmpBuf, name)
		panicIf(e)
		_, e = cmpbin.WriteUint(tmpBuf, uint64(len(vals)))
		panicIf(e)
		for _, p := range vals {
			panicIf(WriteProperty(tmpBuf, context, p))
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
func ReadPropertyMap(buf Buffer, context KeyContext, appid, namespace string) (pm ds.PropertyMap, err error) {
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

		numProps, _, e := cmpbin.ReadUint(buf)
		panicIf(e)
		if numProps > ReadPropertyMapReasonableLimit {
			err = fmt.Errorf("helper: tried to decode map with huge number of properties %d", numProps)
			return
		}
		props := make([]ds.Property, 0, numProps)
		for j := uint64(0); j < numProps; j++ {
			prop, err = ReadProperty(buf, context, appid, namespace)
			panicIf(err)
			props = append(props, prop)
		}
		pm[name] = props
	}
	return
}

// WriteIndexColumn writes an IndexColumn to the buffer.
func WriteIndexColumn(buf Buffer, c ds.IndexColumn) (err error) {
	defer recoverTo(&err)

	if c.Direction == ds.ASCENDING {
		panicIf(buf.WriteByte(0))
	} else {
		panicIf(buf.WriteByte(1))
	}
	_, err = cmpbin.WriteString(buf, c.Property)
	return
}

// ReadIndexColumn reads an IndexColumn from the buffer.
func ReadIndexColumn(buf Buffer) (c ds.IndexColumn, err error) {
	defer recoverTo(&err)

	dir, err := buf.ReadByte()
	panicIf(err)

	switch dir {
	case 0:
		c.Direction = ds.ASCENDING
	default:
		c.Direction = ds.DESCENDING
	}
	c.Property, _, err = cmpbin.ReadString(buf)
	return
}

// WriteIndexDefinition writes an IndexDefinition to the buffer
func WriteIndexDefinition(buf Buffer, i ds.IndexDefinition) (err error) {
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
func ReadIndexDefinition(buf Buffer) (i ds.IndexDefinition, err error) {
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

func toBytesErr(i interface{}, ctx KeyContext) (ret []byte, err error) {
	buf := &bytes.Buffer{}
	switch x := i.(type) {
	case ds.GeoPoint:
		err = WriteGeoPoint(buf, x)

	case ds.IndexColumn:
		err = WriteIndexColumn(buf, x)

	case ds.IndexDefinition:
		err = WriteIndexDefinition(buf, x)

	case ds.Key:
		err = WriteKey(buf, ctx, x)

	case ds.KeyTok:
		err = WriteKeyTok(buf, x)

	case ds.Property:
		err = WriteProperty(buf, ctx, x)

	case ds.PropertyMap:
		err = WritePropertyMap(buf, ctx, x)

	case time.Time:
		err = WriteTime(buf, x)

	default:
		err = fmt.Errorf("unknown type for ToBytes: %T", i)
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
