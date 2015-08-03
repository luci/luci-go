// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/luci/gae/service/blobstore"
	"github.com/luci/luci-go/common/cmpbin"
)

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
const ReadKeyNumToksReasonableLimit uint64 = 50

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
func WriteKey(buf Buffer, context KeyContext, k Key) (err error) {
	// [appid ++ namespace]? ++ #tokens ++ tokens*
	defer recoverTo(&err)
	appid, namespace, toks := KeySplit(k)
	if context == WithContext {
		panicIf(buf.WriteByte(1))
		_, e := cmpbin.WriteString(buf, appid)
		panicIf(e)
		_, e = cmpbin.WriteString(buf, namespace)
		panicIf(e)
	} else {
		panicIf(buf.WriteByte(0))
	}
	_, e := cmpbin.WriteUint(buf, uint64(len(toks)))
	panicIf(e)
	for _, tok := range toks {
		panicIf(WriteKeyTok(buf, tok))
	}
	return nil
}

// ReadKey deserializes a key from the buffer. The value of context must match
// the value of context that was passed to WriteKey when the key was encoded.
// If context == WithoutContext, then the appid and namespace parameters are
// used in the decoded Key. Otherwise they're ignored.
func ReadKey(buf Buffer, context KeyContext, appid, namespace string) (ret Key, err error) {
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

	numToks, _, e := cmpbin.ReadUint(buf)
	panicIf(e)
	if numToks > ReadKeyNumToksReasonableLimit {
		err = fmt.Errorf("helper: tried to decode huge key of length %d", numToks)
		return
	}

	toks := make([]KeyTok, numToks)
	for i := uint64(0); i < numToks; i++ {
		toks[i], e = ReadKeyTok(buf)
		panicIf(e)
	}

	return NewKeyToks(actualAid, actualNS, toks), nil
}

// WriteKeyTok writes a KeyTok to the buffer. You usually want WriteKey
// instead of this.
func WriteKeyTok(buf Buffer, tok KeyTok) (err error) {
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

// ReadKeyTok reads a KeyTok from the buffer. You usually want ReadKey
// instead of this.
func ReadKeyTok(buf Buffer) (ret KeyTok, err error) {
	defer recoverTo(&err)
	e := error(nil)
	ret.Kind, _, e = cmpbin.ReadString(buf)
	panicIf(e)

	typ, e := buf.ReadByte()
	panicIf(e)

	switch PropertyType(typ) {
	case PTString:
		ret.StringID, _, err = cmpbin.ReadString(buf)
	case PTInt:
		ret.IntID, _, err = cmpbin.ReadInt(buf)
		if err == nil && ret.IntID <= 0 {
			err = errors.New("helper: decoded key with empty stringID and zero/negative intID")
		}
	default:
		err = fmt.Errorf("helper: invalid type %s", PropertyType(typ))
	}
	return
}

// Write writes a GeoPoint to the buffer.
func (gp GeoPoint) Write(buf Buffer) (err error) {
	defer recoverTo(&err)
	_, e := cmpbin.WriteFloat64(buf, gp.Lat)
	panicIf(e)
	_, e = cmpbin.WriteFloat64(buf, gp.Lng)
	return e
}

// Read reads a GeoPoint from the buffer.
func (gp *GeoPoint) Read(buf Buffer) (err error) {
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
	_, err := cmpbin.WriteUint(buf, uint64(t.Unix())*1e6+uint64(t.Nanosecond()/1e3))
	return err
}

// ReadTime reads a time.Time from the buffer.
func ReadTime(buf Buffer) (time.Time, error) {
	v, _, err := cmpbin.ReadUint(buf)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(v/1e6), int64((v%1e6)*1e3)).UTC(), nil
}

// Write writes a Property to the buffer. `context` behaves the same
// way that it does for WriteKey, but only has an effect if `p` contains a
// Key as its Value.
func (p *Property) Write(buf Buffer, context KeyContext) (err error) {
	defer recoverTo(&err)
	typb := byte(p.Type())
	if p.IndexSetting() == NoIndex {
		typb |= 0x80
	}
	panicIf(buf.WriteByte(typb))
	switch p.Type() {
	case PTNull, PTBoolTrue, PTBoolFalse:
	case PTInt:
		_, err = cmpbin.WriteInt(buf, p.Value().(int64))
	case PTFloat:
		_, err = cmpbin.WriteFloat64(buf, p.Value().(float64))
	case PTString:
		_, err = cmpbin.WriteString(buf, p.Value().(string))
	case PTBytes:
		if p.IndexSetting() == NoIndex {
			_, err = cmpbin.WriteBytes(buf, p.Value().([]byte))
		} else {
			_, err = cmpbin.WriteBytes(buf, p.Value().(ByteString))
		}
	case PTTime:
		err = WriteTime(buf, p.Value().(time.Time))
	case PTGeoPoint:
		err = p.Value().(GeoPoint).Write(buf)
	case PTKey:
		err = WriteKey(buf, context, p.Value().(Key))
	case PTBlobKey:
		_, err = cmpbin.WriteString(buf, string(p.Value().(blobstore.Key)))
	}
	return
}

// Read reads a Property from the buffer. `context`, `appid`, and
// `namespace` behave the same way they do for ReadKey, but only have an
// effect if the decoded property has a Key value.
func (p *Property) Read(buf Buffer, context KeyContext, appid, namespace string) (err error) {
	val := interface{}(nil)
	typb, err := buf.ReadByte()
	if err != nil {
		return
	}
	is := ShouldIndex
	if (typb & 0x80) != 0 {
		is = NoIndex
	}
	switch PropertyType(typb & 0x7f) {
	case PTNull:
	case PTBoolTrue:
		val = true
	case PTBoolFalse:
		val = false
	case PTInt:
		val, _, err = cmpbin.ReadInt(buf)
	case PTFloat:
		val, _, err = cmpbin.ReadFloat64(buf)
	case PTString:
		val, _, err = cmpbin.ReadString(buf)
	case PTBytes:
		b := []byte(nil)
		if b, _, err = cmpbin.ReadBytes(buf); err != nil {
			break
		}
		if is == NoIndex {
			val = b
		} else {
			val = ByteString(b)
		}
	case PTTime:
		val, err = ReadTime(buf)
	case PTGeoPoint:
		gp := GeoPoint{}
		err = gp.Read(buf)
		val = gp
	case PTKey:
		val, err = ReadKey(buf, context, appid, namespace)
	case PTBlobKey:
		s := ""
		if s, _, err = cmpbin.ReadString(buf); err != nil {
			break
		}
		val = blobstore.Key(s)
	default:
		err = fmt.Errorf("read: unknown type! %v", typb)
	}
	if err == nil {
		err = p.SetValue(val, is)
	}
	return
}

// Write writes an entire PropertyMap to the buffer. `context`
// behaves the same way that it does for WriteKey. If
// WritePropertyMapDeterministic is true, then the rows will be sorted by
// property name before they're serialized to buf (mostly useful for testing,
// but also potentially useful if you need to make a hash of the property data).
func (pm PropertyMap) Write(buf Buffer, context KeyContext) (err error) {
	defer recoverTo(&err)
	rows := make(sort.StringSlice, 0, len(pm))
	tmpBuf := &bytes.Buffer{}
	for name, vals := range pm {
		tmpBuf.Reset()
		_, e := cmpbin.WriteString(tmpBuf, name)
		panicIf(e)
		_, e = cmpbin.WriteUint(tmpBuf, uint64(len(vals)))
		panicIf(e)
		for _, p := range vals {
			panicIf(p.Write(tmpBuf, context))
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

// Read reads a PropertyMap from the buffer. `context` and
// friends behave the same way that they do for ReadKey.
func (pm PropertyMap) Read(buf Buffer, context KeyContext, appid, namespace string) (err error) {
	defer recoverTo(&err)

	numRows := uint64(0)
	numRows, _, e := cmpbin.ReadUint(buf)
	panicIf(e)
	if numRows > ReadPropertyMapReasonableLimit {
		err = fmt.Errorf("helper: tried to decode map with huge number of rows %d", numRows)
		return
	}

	name, prop := "", Property{}
	for i := uint64(0); i < numRows; i++ {
		name, _, e = cmpbin.ReadString(buf)
		panicIf(e)

		numProps, _, e := cmpbin.ReadUint(buf)
		panicIf(e)
		if numProps > ReadPropertyMapReasonableLimit {
			err = fmt.Errorf("helper: tried to decode map with huge number of properties %d", numProps)
			return
		}
		props := make([]Property, 0, numProps)
		for j := uint64(0); j < numProps; j++ {
			panicIf(prop.Read(buf, context, appid, namespace))
			props = append(props, prop)
		}
		pm[name] = props
	}
	return
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
