// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package helper

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"infra/gae/libs/gae"

	"github.com/luci/luci-go/common/cmpbin"
)

// WriteDSPropertyMapDeterministic allows tests to make WriteDSPropertyMap
// deterministic.
var WriteDSPropertyMapDeterministic = false

// ReadDSPropertyMapReasonableLimit sets a limit on the number of rows and
// number of properties per row which can be read by ReadDSPropertyMap. The
// total number of Property objects readable by this method is this number
// squared (e.g. Limit rows * Limit properties)
const ReadDSPropertyMapReasonableLimit uint64 = 30000

// ReadKeyNumToksReasonableLimit is the maximum number of DSKey tokens that
// ReadDSKey is willing to read for a single key.
const ReadKeyNumToksReasonableLimit uint64 = 50

// DSKeyContext controls whether the various Write and Read serializtion
// routines should encode the context of DSKeys (read: the appid and namespace).
// Frequently the appid and namespace of keys are known in advance and so there's
// no reason to redundantly encode them.
type DSKeyContext bool

// With- and WithoutContext indicate if the serialization method should include
// context for DSKeys. See DSKeyContext for more information.
const (
	WithContext    DSKeyContext = true
	WithoutContext              = false
)

// WriteDSKey encodes a key to the buffer. If context is WithContext, then this
// encoded value will include the appid and namespace of the key.
func WriteDSKey(buf *bytes.Buffer, context DSKeyContext, k gae.DSKey) {
	// [appid ++ namespace]? ++ #tokens ++ tokens*
	appid, namespace, toks := DSKeySplit(k)
	if context == WithContext {
		buf.WriteByte(1)
		cmpbin.WriteString(buf, appid)
		cmpbin.WriteString(buf, namespace)
	} else {
		buf.WriteByte(0)
	}
	cmpbin.WriteUint(buf, uint64(len(toks)))
	for _, tok := range toks {
		WriteDSKeyTok(buf, tok)
	}
}

// ReadDSKey deserializes a key from the buffer. The value of context must match
// the value of context that was passed to WriteDSKey when the key was encoded.
// If context == WithoutContext, then the appid and namespace parameters are
// used in the decoded DSKey. Otherwise they're ignored.
func ReadDSKey(buf *bytes.Buffer, context DSKeyContext, appid, namespace string) (ret gae.DSKey, err error) {
	actualCtx, err := buf.ReadByte()
	if err != nil {
		return
	}

	actualAid, actualNS := "", ""
	if actualCtx == 1 {
		if actualAid, _, err = cmpbin.ReadString(buf); err != nil {
			return
		}
		if actualNS, _, err = cmpbin.ReadString(buf); err != nil {
			return
		}
	} else if actualCtx != 0 {
		err = fmt.Errorf("helper: expected actualCtx to be 0 or 1, got %d", actualCtx)
		return
	}

	if context == WithoutContext {
		// overrwrite with the supplied ones
		actualAid = appid
		actualNS = namespace
	}

	numToks := uint64(0)
	if numToks, _, err = cmpbin.ReadUint(buf); err != nil {
		return
	}
	if numToks > ReadKeyNumToksReasonableLimit {
		err = fmt.Errorf("helper: tried to decode huge key of length %d", numToks)
		return
	}

	toks := make([]gae.DSKeyTok, numToks)
	for i := uint64(0); i < numToks; i++ {
		if toks[i], err = ReadDSKeyTok(buf); err != nil {
			return
		}
	}

	return NewDSKeyToks(actualAid, actualNS, toks), nil
}

// WriteDSKeyTok writes a DSKeyTok to the buffer. You usually want WriteDSKey
// instead of this.
func WriteDSKeyTok(buf *bytes.Buffer, tok gae.DSKeyTok) {
	// tok.kind ++ typ ++ [tok.stringID || tok.intID]
	cmpbin.WriteString(buf, tok.Kind)
	if tok.StringID != "" {
		buf.WriteByte(byte(gae.DSPTString))
		cmpbin.WriteString(buf, tok.StringID)
	} else {
		buf.WriteByte(byte(gae.DSPTInt))
		cmpbin.WriteInt(buf, tok.IntID)
	}
}

// ReadDSKeyTok reads a DSKeyTok from the buffer. You usually want ReadDSKey
// instead of this.
func ReadDSKeyTok(buf *bytes.Buffer) (ret gae.DSKeyTok, err error) {
	if ret.Kind, _, err = cmpbin.ReadString(buf); err != nil {
		return
	}
	typ, err := buf.ReadByte()
	if err != nil {
		return
	}
	switch gae.DSPropertyType(typ) {
	case gae.DSPTString:
		ret.StringID, _, err = cmpbin.ReadString(buf)
	case gae.DSPTInt:
		ret.IntID, _, err = cmpbin.ReadInt(buf)
		if err == nil && ret.IntID <= 0 {
			err = errors.New("helper: decoded key with empty stringID and zero/negative intID")
		}
	default:
		err = fmt.Errorf("helper: invalid type %s", gae.DSPropertyType(typ))
	}
	return
}

// WriteDSGeoPoint writes a DSGeoPoint to the buffer.
func WriteDSGeoPoint(buf *bytes.Buffer, gp gae.DSGeoPoint) {
	cmpbin.WriteFloat64(buf, gp.Lat)
	cmpbin.WriteFloat64(buf, gp.Lng)
}

// ReadDSGeoPoint reads a DSGeoPoint from the buffer.
func ReadDSGeoPoint(buf *bytes.Buffer) (gp gae.DSGeoPoint, err error) {
	if gp.Lat, _, err = cmpbin.ReadFloat64(buf); err != nil {
		return
	}
	gp.Lng, _, err = cmpbin.ReadFloat64(buf)
	if err == nil && !gp.Valid() {
		err = fmt.Errorf("helper: decoded invalid DSGeoPoint: %v", gp)
	}
	return
}

// WriteTime writes a time.Time in a byte-sortable way.
//
// This method truncates the time to microseconds and drops the timezone,
// because that's the (undocumented) way that the appengine SDK does it.
func WriteTime(buf *bytes.Buffer, t time.Time) {
	name, off := t.Zone()
	if name != "UTC" || off != 0 {
		panic(fmt.Errorf("helper: UTC OR DEATH: %s", t))
	}
	cmpbin.WriteUint(buf, uint64(t.Unix())*1e6+uint64(t.Nanosecond()/1e3))
}

// ReadTime reads a time.Time from the buffer.
func ReadTime(buf *bytes.Buffer) (time.Time, error) {
	v, _, err := cmpbin.ReadUint(buf)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(int64(v/1e6), int64((v%1e6)*1e3)).UTC(), nil
}

// WriteDSProperty writes a DSProperty to the buffer. `context` behaves the same
// way that it does for WriteDSKey, but only has an effect if `p` contains a
// DSKey as its Value.
func WriteDSProperty(buf *bytes.Buffer, p gae.DSProperty, context DSKeyContext) {
	typb := byte(p.Type())
	if p.IndexSetting() == gae.NoIndex {
		typb |= 0x80
	}
	buf.WriteByte(typb)
	switch p.Type() {
	case gae.DSPTNull, gae.DSPTBoolTrue, gae.DSPTBoolFalse:
	case gae.DSPTInt:
		cmpbin.WriteInt(buf, p.Value().(int64))
	case gae.DSPTFloat:
		cmpbin.WriteFloat64(buf, p.Value().(float64))
	case gae.DSPTString:
		cmpbin.WriteString(buf, p.Value().(string))
	case gae.DSPTBytes:
		if p.IndexSetting() == gae.NoIndex {
			cmpbin.WriteBytes(buf, p.Value().([]byte))
		} else {
			cmpbin.WriteBytes(buf, p.Value().(gae.DSByteString))
		}
	case gae.DSPTTime:
		WriteTime(buf, p.Value().(time.Time))
	case gae.DSPTGeoPoint:
		WriteDSGeoPoint(buf, p.Value().(gae.DSGeoPoint))
	case gae.DSPTKey:
		WriteDSKey(buf, context, p.Value().(gae.DSKey))
	case gae.DSPTBlobKey:
		cmpbin.WriteString(buf, string(p.Value().(gae.BSKey)))
	}
}

// ReadDSProperty reads a DSProperty from the buffer. `context`, `appid`, and
// `namespace` behave the same way they do for ReadDSKey, but only have an
// effect if the decoded property has a DSKey value.
func ReadDSProperty(buf *bytes.Buffer, context DSKeyContext, appid, namespace string) (p gae.DSProperty, err error) {
	val := interface{}(nil)
	typb, err := buf.ReadByte()
	if err != nil {
		return
	}
	is := gae.ShouldIndex
	if (typb & 0x80) != 0 {
		is = gae.NoIndex
	}
	switch gae.DSPropertyType(typb & 0x7f) {
	case gae.DSPTNull:
	case gae.DSPTBoolTrue:
		val = true
	case gae.DSPTBoolFalse:
		val = false
	case gae.DSPTInt:
		val, _, err = cmpbin.ReadInt(buf)
	case gae.DSPTFloat:
		val, _, err = cmpbin.ReadFloat64(buf)
	case gae.DSPTString:
		val, _, err = cmpbin.ReadString(buf)
	case gae.DSPTBytes:
		b := []byte(nil)
		if b, _, err = cmpbin.ReadBytes(buf); err != nil {
			break
		}
		if is == gae.NoIndex {
			val = b
		} else {
			val = gae.DSByteString(b)
		}
	case gae.DSPTTime:
		val, err = ReadTime(buf)
	case gae.DSPTGeoPoint:
		val, err = ReadDSGeoPoint(buf)
	case gae.DSPTKey:
		val, err = ReadDSKey(buf, context, appid, namespace)
	case gae.DSPTBlobKey:
		s := ""
		if s, _, err = cmpbin.ReadString(buf); err != nil {
			break
		}
		val = gae.BSKey(s)
	default:
		err = fmt.Errorf("read: unknown type! %v", typb)
	}
	if err == nil {
		err = p.SetValue(val, is)
	}
	return
}

// WriteDSPropertyMap writes an entire DSPropertyMap to the buffer. `context`
// behaves the same way that it does for WriteDSKey. If
// WriteDSPropertyMapDeterministic is true, then the rows will be sorted by
// property name before they're serialized to buf (mostly useful for testing,
// but also potentially useful if you need to make a hash of the property data).
func WriteDSPropertyMap(buf *bytes.Buffer, propMap gae.DSPropertyMap, context DSKeyContext) {
	rows := make(sort.StringSlice, 0, len(propMap))
	tmpBuf := &bytes.Buffer{}
	for name, vals := range propMap {
		tmpBuf.Reset()
		cmpbin.WriteString(tmpBuf, name)
		cmpbin.WriteUint(tmpBuf, uint64(len(vals)))
		for _, p := range vals {
			WriteDSProperty(tmpBuf, p, context)
		}
		rows = append(rows, tmpBuf.String())
	}

	if WriteDSPropertyMapDeterministic {
		rows.Sort()
	}

	cmpbin.WriteUint(buf, uint64(len(propMap)))
	for _, r := range rows {
		buf.WriteString(r)
	}
}

// ReadDSPropertyMap reads a DSPropertyMap from the buffer. `context` and
// friends behave the same way that they do for ReadDSKey.
func ReadDSPropertyMap(buf *bytes.Buffer, context DSKeyContext, appid, namespace string) (propMap gae.DSPropertyMap, err error) {
	numRows := uint64(0)
	if numRows, _, err = cmpbin.ReadUint(buf); err != nil {
		return
	}
	if numRows > ReadDSPropertyMapReasonableLimit {
		err = fmt.Errorf("helper: tried to decode map with huge number of rows %d", numRows)
		return
	}

	name, prop := "", gae.DSProperty{}
	propMap = make(gae.DSPropertyMap, numRows)
	for i := uint64(0); i < numRows; i++ {
		if name, _, err = cmpbin.ReadString(buf); err != nil {
			return
		}

		numProps := uint64(0)
		if numProps, _, err = cmpbin.ReadUint(buf); err != nil {
			return
		}
		if numProps > ReadDSPropertyMapReasonableLimit {
			err = fmt.Errorf("helper: tried to decode map with huge number of properties %d", numProps)
			return
		}
		props := make([]gae.DSProperty, 0, numProps)
		for j := uint64(0); j < numProps; j++ {
			if prop, err = ReadDSProperty(buf, context, appid, namespace); err != nil {
				return
			}
			props = append(props, prop)
		}
		propMap[name] = props
	}
	return
}
