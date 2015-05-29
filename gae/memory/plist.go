// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"reflect"
	"time"

	"github.com/luci/luci-go/common/funnybase"

	"appengine"
	"appengine/datastore"
)

type typData struct {
	noIndex bool
	typ     propValType
	data    interface{}
}

func newTypData(noIndex bool, v interface{}) (ret *typData, err error) {
	typ := pvUNKNOWN

	switch x := v.(type) {
	case nil:
		typ = pvNull
	case time.Time:
		typ = pvTime
	case int64:
		typ = pvInt
	case float64:
		typ = pvFloat
	case bool:
		if x {
			typ = pvBoolTrue
		} else {
			typ = pvBoolFalse
		}
	case []byte, datastore.ByteString:
		typ = pvBytes
	case appengine.BlobKey:
		typ = pvBlobKey
	case string:
		typ = pvStr
	case appengine.GeoPoint:
		typ = pvGeoPoint
	case *datastore.Key:
		typ = pvKey
	}
	if typ == pvUNKNOWN {
		err = fmt.Errorf("propValTypeOf: unknown type of %#v", v)
	}

	return &typData{noIndex, typ, v}, err
}

func (td *typData) WriteBinary(buf *bytes.Buffer) error {
	typb := byte(td.typ)
	if td.noIndex {
		typb |= 0x80
	}
	buf.WriteByte(typb)
	switch td.typ {
	case pvNull, pvBoolFalse, pvBoolTrue:
		return nil
	case pvInt:
		funnybase.Write(buf, td.data.(int64))
	case pvFloat:
		writeFloat64(buf, td.data.(float64))
	case pvStr:
		writeString(buf, td.data.(string))
	case pvBytes:
		if td.noIndex {
			writeBytes(buf, td.data.([]byte))
		} else {
			writeBytes(buf, td.data.(datastore.ByteString))
		}
	case pvTime:
		t := td.data.(time.Time)
		funnybase.WriteUint(buf, uint64(t.Unix())*1e6+uint64(t.Nanosecond()/1e3))
	case pvGeoPoint:
		t := td.data.(appengine.GeoPoint)
		writeFloat64(buf, t.Lat)
		writeFloat64(buf, t.Lng)
	case pvKey:
		writeKey(buf, withNS, td.data.(*datastore.Key))
	case pvBlobKey:
		writeString(buf, string(td.data.(appengine.BlobKey)))
	default:
		return fmt.Errorf("write: unknown type! %v", td)
	}
	return nil
}

func (td *typData) ReadBinary(buf *bytes.Buffer) error {
	typb, err := buf.ReadByte()
	if err != nil {
		return err
	}
	td.noIndex = (typb & 0x80) != 0 // highbit means noindex
	td.typ = propValType(typb & 0x7f)
	switch td.typ {
	case pvNull:
		td.data = nil
	case pvBoolTrue:
		td.data = true
	case pvBoolFalse:
		td.data = false
	case pvInt:
		v, err := funnybase.Read(buf)
		if err != nil {
			return err
		}
		td.data = v
	case pvFloat:
		td.data, err = readFloat64(buf)
		if err != nil {
			return err
		}
	case pvStr:
		td.data, err = readString(buf)
		if err != nil {
			return err
		}
	case pvBytes:
		b, err := readBytes(buf)
		if err != nil {
			return err
		}
		if td.noIndex {
			td.data = b
		} else {
			td.data = datastore.ByteString(b)
		}
	case pvTime:
		v, err := funnybase.ReadUint(buf)
		if err != nil {
			return err
		}
		td.data = time.Unix(int64(v/1e6), int64((v%1e6)*1e3))
	case pvGeoPoint:
		pt := appengine.GeoPoint{}
		pt.Lat, err = readFloat64(buf)
		if err != nil {
			return err
		}
		pt.Lng, err = readFloat64(buf)
		if err != nil {
			return err
		}
		td.data = pt
	case pvKey:
		td.data, err = readKey(buf, true)
		if err != nil {
			return err
		}
	case pvBlobKey:
		s, err := readString(buf)
		if err != nil {
			return err
		}
		td.data = appengine.BlobKey(s)
	default:
		return fmt.Errorf("read: unknown type! %v", td)
	}

	return nil
}

type pval struct {
	name  string
	multi bool
	vals  []*typData
}

type propertyList []datastore.Property

var _ = datastore.PropertyLoadSaver((*propertyList)(nil))

func (pl *propertyList) Load(ch <-chan datastore.Property) error {
	return (*datastore.PropertyList)(pl).Load(ch)
}

func (pl *propertyList) Save(ch chan<- datastore.Property) error {
	return (*datastore.PropertyList)(pl).Save(ch)
}

func (pl *propertyList) collate() ([]*pval, error) {
	if pl == nil || len(*pl) == 0 {
		return nil, nil
	}

	cols := []*pval{}
	colIdx := map[string]int{}

	for _, p := range *pl {
		if idx, ok := colIdx[p.Name]; !ok {
			colIdx[p.Name] = len(cols)
			td, err := newTypData(p.NoIndex, p.Value)
			if err != nil {
				return nil, err
			}
			cols = append(cols, &pval{p.Name, p.Multiple, []*typData{td}})
		} else {
			c := cols[idx]
			if c.multi != p.Multiple {
				return nil, fmt.Errorf(
					"propertyList.MarshalBinary: field %q has conflicting values of Multiple", p.Name)
			}
			td, err := newTypData(p.NoIndex, p.Value)
			if err != nil {
				return nil, err
			}
			c.vals = append(c.vals, td)
		}
	}

	return cols, nil
}

func (pl *propertyList) addCollated(pv *pval) {
	for _, v := range pv.vals {
		*pl = append(*pl, datastore.Property{
			Name:     pv.name,
			Multiple: pv.multi,
			NoIndex:  v.noIndex,
			Value:    v.data,
		})
	}
}

func (pl *propertyList) MarshalBinary() ([]byte, error) {
	cols, err := pl.collate()
	if err != nil || len(cols) == 0 {
		return nil, err
	}

	pieces := make([][]byte, 0, len(*pl)*2+1)
	for _, pv := range cols {
		// TODO(riannucci): estimate buffer size better.
		buf := bytes.NewBuffer(make([]byte, 0, funnybase.MaxFunnyBaseLen64+len(pv.name)))
		writeString(buf, pv.name)
		err := pv.WriteBinary(buf)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, buf.Bytes())
	}
	return bytes.Join(pieces, nil), nil
}

func (pl *propertyList) UnmarshalBinary(data []byte) error {
	buf := bytes.NewBuffer(data)
	for buf.Len() > 0 {
		name, err := readString(buf)
		if err != nil {
			return err
		}

		pv := &pval{name: name}
		err = pv.ReadBinary(buf)
		if err != nil {
			return err
		}
		pl.addCollated(pv)
	}

	return nil
}

func toPL(src interface{}) (ret *propertyList, err error) {
	propchan := make(chan datastore.Property)
	ret = &propertyList{}
	go func() { err = datastore.SaveStruct(src, propchan) }()
	err2 := ret.Load(propchan)
	if err != nil {
		return
	}
	return ret, err2
}

func fromPL(props *propertyList, dst interface{}) (err error) {
	propchan := make(chan datastore.Property)
	go func() { err = props.Save(propchan) }()
	err2 := datastore.LoadStruct(dst, propchan)
	if err != nil {
		return err
	}
	return err2
}

type propValType byte

var byteSliceType = reflect.TypeOf([]byte(nil))

// These constants are in the order described by
//   https://cloud.google.com/appengine/docs/go/datastore/entities#Go_Value_type_ordering
// with a slight divergence for the Int/Time split.
const (
	pvNull propValType = iota
	pvInt

	// NOTE: this is a slight divergence; times and integers actually sort
	// together (apparently?) in datastore. This is probably insane, and I don't
	// want to add the complexity of field 'meaning' as a sparate concept from the
	// field's 'type' (which is what datastore seems to do, judging from the
	// protobufs). So if you're here because you implemented an app which relies
	// on time.Time and int64 sorting together, then this is why your app acts
	// differently in production. My advice is to NOT DO THAT. If you really want
	// this (and you probably don't), you should take care of the time.Time <->
	// int64 conversion in your app and just use a property type of int64.
	pvTime

	// NOTE: this is also a slight divergence, but not a semantic one. IIUC, in
	// datastore 'bool' is actually the type and the value is either 0 or
	// 1 (taking another byte to store). Since we have plenty of space in this
	// type byte, I just merge the value into the type for booleans. If this
	// becomes problematic, consider changing this to just pvBool, and then
	// encoding a 0 or 1 as a byte in the relevant marshalling routines.
	pvBoolFalse
	pvBoolTrue
	pvBytes // []byte or datastore.ByteString
	pvStr   // string or string noindex
	pvFloat
	pvGeoPoint

	// These two are problematic, because they force us to bind to the appengine
	// SDK code. If we can drop support for these and turn them into hard errors,
	// that could let us decouple from the various appengine SDKs. Maybe.
	pvKey     // TODO(riannucci): remove support for this  (use a string)
	pvBlobKey // TODO(riannucci): remove support for this  (use a string)

	pvUNKNOWN
)

func (p *pval) ReadBinary(buf *bytes.Buffer) error {
	n, err := funnybase.ReadUint(buf)
	if err != nil {
		return err
	}
	p.multi = n > 1

	p.vals = make([]*typData, n)
	for i := range p.vals {
		p.vals[i] = &typData{}
		err := p.vals[i].ReadBinary(buf)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *pval) WriteBinary(buf *bytes.Buffer) error {
	funnybase.WriteUint(buf, uint64(len(p.vals)))
	for _, v := range p.vals {
		if err := v.WriteBinary(buf); err != nil {
			return err
		}
	}
	return nil
}
