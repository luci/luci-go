// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"infra/gae/libs/gae"

	. "github.com/smartystreets/goconvey/convey"
)

func mp(value interface{}, noIndexes ...bool) (ret gae.DSProperty) {
	ni := false
	if len(noIndexes) > 1 {
		panic("YOU FOOL! YOU CANNOT HOPE TO PASS THAT MANY VALUES!")
	} else if len(noIndexes) == 1 {
		ni = noIndexes[0]
	}
	if err := ret.SetValue(value, ni); err != nil {
		panic(err)
	}
	return
}

const testAppID = "testApp"

type (
	myBlob   []byte
	myByte   byte
	myString string
)

func makeMyByteSlice(n int) []myByte {
	b := make([]myByte, n)
	for i := range b {
		b[i] = myByte(i)
	}
	return b
}

func makeInt8Slice(n int) []int8 {
	b := make([]int8, n)
	for i := range b {
		b[i] = int8(i)
	}
	return b
}

func makeUint8Slice(n int) []uint8 {
	b := make([]uint8, n)
	for i := range b {
		b[i] = uint8(i)
	}
	return b
}

var (
	testKey0     = mkKey("aid", "", "kind", "name0")
	testKey1a    = mkKey("aid", "", "kind", "name1")
	testKey1b    = mkKey("aid", "", "kind", "name1")
	testKey2a    = mkKey("aid", "", "kind", "name0", "kind", "name2")
	testKey2b    = mkKey("aid", "", "kind", "name0", "kind", "name2")
	testGeoPt0   = gae.DSGeoPoint{Lat: 1.2, Lng: 3.4}
	testGeoPt1   = gae.DSGeoPoint{Lat: 5, Lng: 10}
	testBadGeoPt = gae.DSGeoPoint{Lat: 1000, Lng: 34}
)

type B0 struct {
	B []byte
}

type B1 struct {
	B []int8
}

type B2 struct {
	B myBlob
}

type B3 struct {
	B []myByte
}

type B4 struct {
	B [][]byte
}

type B5 struct {
	B gae.DSByteString
}

type C0 struct {
	I int
	C chan int
}

type C1 struct {
	I int
	C *chan int
}

type C2 struct {
	I int
	C []chan int
}

type C3 struct {
	C string
}

type E struct{}

type G0 struct {
	G gae.DSGeoPoint
}

type G1 struct {
	G []gae.DSGeoPoint
}

type K0 struct {
	K gae.DSKey
}

type K1 struct {
	K []gae.DSKey
}

type N0 struct {
	X0
	ID       int64  `gae:"$id"`
	_kind    string `gae:"$kind,whatnow"`
	Nonymous X0
	Ignore   string `gae:"-"`
	Other    string
}

type N1 struct {
	X0
	Nonymous []X0
	Ignore   string `gae:"-"`
	Other    string
}

type N2 struct {
	N1    `gae:"red"`
	Green N1 `gae:"green"`
	Blue  N1
	White N1 `gae:"-"`
}

type O0 struct {
	I int64
}

type O1 struct {
	I int32
}

type U0 struct {
	U uint
}

type U1 struct {
	U string
}

type T struct {
	T time.Time
}

type X0 struct {
	S string
	I int
	i int
}

type X1 struct {
	S myString
	I int32
	J int64
}

type X2 struct {
	Z string
	i int
}

type X3 struct {
	S bool
	I int
}

type Y0 struct {
	B bool
	F []float64
	G []float64
}

type Y1 struct {
	B bool
	F float64
}

type Y2 struct {
	B bool
	F []int64
}

type Y3 struct {
	B bool
	F int64
}

type Tagged struct {
	A int   `gae:"a,noindex"`
	B []int `gae:"b1"`
	C int   `gae:",noindex"`
	D int   `gae:""`
	E int
	I int `gae:"-"`
	J int `gae:",noindex" json:"j"`

	Y0 `gae:"-"`
	Z  chan int `gae:"-,"`
}

type InvalidTagged1 struct {
	I int `gae:"\t"`
}

type InvalidTagged2 struct {
	I int
	J int `gae:"I"`
}

type InvalidTagged3 struct {
	I int `gae:"a\t"`
}

type InvalidTagged4 struct {
	I int `gae:"a."`
}

type InvalidTaggedSub struct {
	I int
}

type InvalidTagged5 struct {
	I int `gae:"V.I"`
	V []InvalidTaggedSub
}

type Inner1 struct {
	W int32
	X string
}

type Inner2 struct {
	Y float64
}

type Inner3 struct {
	Z bool
}

type Outer struct {
	A int16
	I []Inner1
	J Inner2
	Inner3
}

type OuterEquivalent struct {
	A     int16
	IDotW []int32  `gae:"I.W"`
	IDotX []string `gae:"I.X"`
	JDotY float64  `gae:"J.Y"`
	Z     bool
}

type Dotted struct {
	A DottedA `gae:"A0.A1.A2"`
}

type DottedA struct {
	B DottedB `gae:"B3"`
}

type DottedB struct {
	C int `gae:"C4.C5"`
}

type SliceOfSlices struct {
	I int
	S []struct {
		J int
		F []float64
	}
}

type Recursive struct {
	I int
	R []Recursive
}

type MutuallyRecursive0 struct {
	I int
	R []MutuallyRecursive1
}

type MutuallyRecursive1 struct {
	I int
	R []MutuallyRecursive0
}

type ExoticTypes struct {
	BS   gae.BSKey
	DSBS gae.DSByteString
}

type Underspecified struct {
	Iface gae.DSPropertyConverter
}

type MismatchTypes struct {
	S  string
	B  bool
	F  float32
	K  gae.DSKey
	T  time.Time
	G  gae.DSGeoPoint
	IS []int
}

type BadSpecial struct {
	ID int    `gae:"$id"`
	id string `gae:"$id"`
}

type Doubler struct {
	S string
	I int64
	B bool
}

func (d *Doubler) Load(props gae.DSPropertyMap) ([]string, error) {
	return GetStructPLS(d).Load(props)
}

func (d *Doubler) Save() (gae.DSPropertyMap, error) {
	// Save the default gae.DSProperty slice to an in-memory buffer (a gae.DSPropertyList).
	pls := GetStructPLS(d)
	props, err := pls.Save()
	if err != nil {
		return nil, err
	}
	var propMap gae.DSPropertyMap
	propMap.Load(props) // we know this returns nil/nil

	// Edit that map and send it on.
	for _, props := range propMap {
		for i := range props {
			switch v := props[i].Value().(type) {
			case string:
				// + means string concatenation.
				props[i].SetValue(v+v, props[i].NoIndex())
			case int64:
				// + means integer addition.
				props[i].SetValue(v+v, props[i].NoIndex())
			}
		}
	}
	return propMap.Save()
}

var _ gae.DSPropertyLoadSaver = (*Doubler)(nil)

type Deriver struct {
	S, Derived, Ignored string
}

func (e *Deriver) Load(props gae.DSPropertyMap) ([]string, error) {
	for name, p := range props {
		if name != "S" {
			continue
		}
		e.S = p[0].Value().(string)
		e.Derived = "derived+" + e.S
	}
	return nil, nil
}

func (e *Deriver) Save() (gae.DSPropertyMap, error) {
	return map[string][]gae.DSProperty{
		"S": {mp(e.S)},
	}, nil
}

var _ gae.DSPropertyLoadSaver = (*Deriver)(nil)

type BK struct {
	Key gae.BSKey
}

type Convertable []int64

var _ gae.DSPropertyConverter = (*Convertable)(nil)

func (c *Convertable) ToDSProperty() (ret gae.DSProperty, err error) {
	buf := make([]string, len(*c))
	for i, v := range *c {
		buf[i] = strconv.FormatInt(v, 10)
	}
	err = ret.SetValue(strings.Join(buf, ","), true)
	return
}

func (c *Convertable) FromDSProperty(pv gae.DSProperty) error {
	if sval, ok := pv.Value().(string); ok {
		for _, t := range strings.Split(sval, ",") {
			ival, err := strconv.ParseInt(t, 10, 64)
			if err != nil {
				return err
			}
			*c = append(*c, ival)
		}
		return nil
	}
	return fmt.Errorf("nope")
}

type Impossible struct {
	Nested []ImpossibleInner
}

type ImpossibleInner struct {
	Ints Convertable `gae:"wot"`
}

type Convertable2 struct {
	Data     string
	Exploded []string
}

func (c *Convertable2) ToDSProperty() (ret gae.DSProperty, err error) {
	err = ret.SetValue(c.Data, false)
	return
}

func (c *Convertable2) FromDSProperty(pv gae.DSProperty) error {
	if sval, ok := pv.Value().(string); ok {
		c.Data = sval
		c.Exploded = []string{"turn", "down", "for", "what"}
		return nil
	}
	return fmt.Errorf("nope")
}

type Impossible2 struct {
	Nested []ImpossibleInner2
}

type ImpossibleInner2 struct {
	Thingy Convertable2 `gae:"nerb"`
}

type JSONKVProp map[string]interface{}

var _ gae.DSPropertyConverter = (*JSONKVProp)(nil)

func (j *JSONKVProp) ToDSProperty() (ret gae.DSProperty, err error) {
	data, err := json.Marshal(map[string]interface{}(*j))
	if err != nil {
		return
	}
	err = ret.SetValue(data, true)
	return
}

func (j *JSONKVProp) FromDSProperty(pv gae.DSProperty) error {
	if bval, ok := pv.Value().([]byte); ok {
		dec := json.NewDecoder(bytes.NewBuffer(bval))
		dec.UseNumber()
		return dec.Decode((*map[string]interface{})(j))
	}
	return fmt.Errorf("nope")
}

type Impossible3 struct {
	KMap JSONKVProp `gae:"kewelmap"`
}

type Complex complex128

var _ gae.DSPropertyConverter = (*Complex)(nil)

func (c *Complex) ToDSProperty() (ret gae.DSProperty, err error) {
	// cheat hardkore and usurp GeoPoint so datastore will index these suckers
	// (note that this won't REALLY work, since GeoPoints are limited to a very
	// limited range of values, but it's nice to pretend ;)). You'd probably
	// really end up with a packed binary representation.
	err = ret.SetValue(gae.DSGeoPoint{Lat: real(*c), Lng: imag(*c)}, false)
	return
}

func (c *Complex) FromDSProperty(p gae.DSProperty) error {
	if gval, ok := p.Value().(gae.DSGeoPoint); ok {
		*c = Complex(complex(gval.Lat, gval.Lng))
		return nil
	}
	return fmt.Errorf("nope")
}

type Impossible4 struct {
	Values []Complex
}

type DerivedKey struct {
	K *GenericDSKey
}

type IfaceKey struct {
	K gae.DSKey
}

type testCase struct {
	desc          string
	src           interface{}
	want          interface{}
	plsErr        string
	saveErr       string
	actualNoIndex bool
	plsLoadErr    string
	convErr       string
	loadErr       string
}

var testCases = []testCase{
	{
		desc:   "chan save fails",
		src:    &C0{I: -1},
		plsErr: `field "C" has invalid type: chan int`,
	},
	{
		desc:   "*chan save fails",
		src:    &C1{I: -1},
		plsErr: `field "C" has invalid type: *chan int`,
	},
	{
		desc:   "[]chan save fails",
		src:    &C2{I: -1, C: make([]chan int, 8)},
		plsErr: `field "C" has invalid type: []chan int`,
	},
	{
		desc:       "chan load fails",
		src:        &C3{C: "not a chan"},
		want:       &C0{},
		plsLoadErr: `field "C" has invalid type: chan int`,
	},
	{
		desc:       "*chan load fails",
		src:        &C3{C: "not a *chan"},
		want:       &C1{},
		plsLoadErr: `field "C" has invalid type: *chan int`,
	},
	{
		desc:       "[]chan load fails",
		src:        &C3{C: "not a []chan"},
		want:       &C2{},
		plsLoadErr: `field "C" has invalid type: []chan int`,
	},
	{
		desc: "empty struct",
		src:  &E{},
		want: &E{},
	},
	{
		desc: "geopoint",
		src:  &G0{G: testGeoPt0},
		want: &G0{G: testGeoPt0},
	},
	{
		desc:    "geopoint invalid",
		src:     &G0{G: testBadGeoPt},
		saveErr: "invalid GeoPoint value",
	},
	{
		desc: "geopoint as props",
		src:  &G0{G: testGeoPt0},
		want: &gae.DSPropertyMap{
			"G": {mp(testGeoPt0)},
		},
	},
	{
		desc: "geopoint slice",
		src:  &G1{G: []gae.DSGeoPoint{testGeoPt0, testGeoPt1}},
		want: &G1{G: []gae.DSGeoPoint{testGeoPt0, testGeoPt1}},
	},
	{
		desc: "key",
		src:  &K0{K: testKey1a},
		want: &K0{K: testKey1b},
	},
	{
		desc: "key with parent",
		src:  &K0{K: testKey2a},
		want: &K0{K: testKey2b},
	},
	{
		desc: "nil key",
		src:  &K0{},
		want: &K0{},
	},
	{
		desc: "all nil keys in slice",
		src:  &K1{[]gae.DSKey{nil, nil}},
		want: &K1{[]gae.DSKey{nil, nil}},
	},
	{
		desc: "some nil keys in slice",
		src:  &K1{[]gae.DSKey{testKey1a, nil, testKey2a}},
		want: &K1{[]gae.DSKey{testKey1b, nil, testKey2b}},
	},
	{
		desc:    "overflow",
		src:     &O0{I: 1 << 48},
		want:    &O1{},
		convErr: "overflow",
	},
	{
		desc: "time",
		src:  &T{T: time.Unix(1e9, 0).UTC()},
		want: &T{T: time.Unix(1e9, 0).UTC()},
	},
	{
		desc: "time as props",
		src:  &T{T: time.Unix(1e9, 0).UTC()},
		want: &gae.DSPropertyMap{
			"T": {mp(time.Unix(1e9, 0).UTC(), false)},
		},
	},
	{
		desc:   "uint save",
		src:    &U0{U: 1},
		plsErr: `field "U" has invalid type: uint`,
	},
	{
		desc:       "uint load",
		src:        &U1{U: "not a uint"},
		want:       &U0{},
		plsLoadErr: `field "U" has invalid type: uint`,
	},
	{
		desc: "zero",
		src:  &X0{},
		want: &X0{},
	},
	{
		desc: "basic",
		src:  &X0{S: "one", I: 2, i: 3},
		want: &X0{S: "one", I: 2},
	},
	{
		desc: "save string/int load myString/int32",
		src:  &X0{S: "one", I: 2, i: 3},
		want: &X1{S: "one", I: 2},
	},
	{
		desc:    "missing fields",
		src:     &X0{S: "one", I: 2, i: 3},
		want:    &X2{},
		convErr: "no such struct field",
	},
	{
		desc:    "save string load bool",
		src:     &X0{S: "one", I: 2, i: 3},
		want:    &X3{I: 2},
		convErr: "type mismatch",
	},
	{
		desc: "basic slice",
		src:  &Y0{B: true, F: []float64{7, 8, 9}},
		want: &Y0{B: true, F: []float64{7, 8, 9}},
	},
	{
		desc:    "save []float64 load float64",
		src:     &Y0{B: true, F: []float64{7, 8, 9}},
		want:    &Y1{B: true},
		convErr: "requires a slice",
	},
	{
		desc: "save single []int64 load int64",
		src:  &Y2{B: true, F: []int64{7}},
		want: &Y3{B: true, F: 7},
	},
	{
		desc: "save int64 load single []int64",
		src:  &Y3{B: true, F: 7},
		want: &Y2{B: true, F: []int64{7}},
	},
	{
		desc: "use convertable slice",
		src:  &Impossible{[]ImpossibleInner{{Convertable{1, 5, 9}}, {Convertable{2, 4, 6}}}},
		want: &Impossible{[]ImpossibleInner{{Convertable{1, 5, 9}}, {Convertable{2, 4, 6}}}},
	},
	{
		desc: "use convertable slice (to map)",
		src:  &Impossible{[]ImpossibleInner{{Convertable{1, 5, 9}}, {Convertable{2, 4, 6}}}},
		want: &gae.DSPropertyMap{
			"Nested.wot": {mp("1,5,9", true), mp("2,4,6", true)},
		},
	},
	{
		desc:    "convertable slice (bad load)",
		src:     &gae.DSPropertyMap{"Nested.wot": {mp([]byte("ohai"), true)}},
		want:    &Impossible{[]ImpossibleInner{{}}},
		convErr: "nope",
	},
	{
		desc: "use convertable struct",
		src: &Impossible2{
			[]ImpossibleInner2{
				{Convertable2{"nerb", nil}},
			},
		},
		want: &Impossible2{
			[]ImpossibleInner2{
				{Convertable2{"nerb", []string{"turn", "down", "for", "what"}}},
			},
		},
	},
	{
		desc: "convertable json KVMap",
		src: &Impossible3{
			JSONKVProp{
				"epic":    "success",
				"no_way!": []interface{}{true, "story"},
				"what":    []interface{}{"is", "really", 100},
			},
		},
		want: &Impossible3{
			JSONKVProp{
				"epic":    "success",
				"no_way!": []interface{}{true, "story"},
				"what":    []interface{}{"is", "really", json.Number("100")},
			},
		},
	},
	{
		desc: "convertable json KVMap (to map)",
		src: &Impossible3{
			JSONKVProp{
				"epic":    "success",
				"no_way!": []interface{}{true, "story"},
				"what":    []interface{}{"is", "really", 100},
			},
		},
		want: &gae.DSPropertyMap{
			"kewelmap": {
				mp([]byte(
					`{"epic":"success","no_way!":[true,"story"],"what":["is","really",100]}`),
					true)},
		},
	},
	{
		desc: "convertable complex slice",
		src: &Impossible4{
			[]Complex{complex(1, 2), complex(3, 4)},
		},
		want: &Impossible4{
			[]Complex{complex(1, 2), complex(3, 4)},
		},
	},
	{
		desc: "convertable complex slice (to map)",
		src: &Impossible4{
			[]Complex{complex(1, 2), complex(3, 4)},
		},
		want: &gae.DSPropertyMap{
			"Values": {
				mp(gae.DSGeoPoint{Lat: 1, Lng: 2}), mp(gae.DSGeoPoint{Lat: 3, Lng: 4})},
		},
	},
	{
		desc:    "convertable complex slice (bad load)",
		src:     &gae.DSPropertyMap{"Values": {mp("hello")}},
		want:    &Impossible4{[]Complex(nil)},
		convErr: "nope",
	},
	{
		desc: "allow concrete gae.DSKey implementors (save)",
		src:  &DerivedKey{testKey2a.(*GenericDSKey)},
		want: &IfaceKey{testKey2b},
	},
	{
		desc: "allow concrete gae.DSKey implementors (load)",
		src:  &IfaceKey{testKey2b},
		want: &DerivedKey{testKey2a.(*GenericDSKey)},
	},
	{
		desc:    "save []float64 load []int64",
		src:     &Y0{B: true, F: []float64{7, 8, 9}},
		want:    &Y2{B: true},
		convErr: "type mismatch",
	},
	{
		desc:    "single slice is too long",
		src:     &Y0{F: make([]float64, maxIndexedProperties+1)},
		want:    &Y0{},
		saveErr: "gae: too many indexed properties",
	},
	{
		desc:    "two slices are too long",
		src:     &Y0{F: make([]float64, maxIndexedProperties), G: make([]float64, maxIndexedProperties)},
		want:    &Y0{},
		saveErr: "gae: too many indexed properties",
	},
	{
		desc:    "one slice and one scalar are too long",
		src:     &Y0{F: make([]float64, maxIndexedProperties), B: true},
		want:    &Y0{},
		saveErr: "gae: too many indexed properties",
	},
	{
		desc: "long blob",
		src:  &B0{B: makeUint8Slice(maxIndexedProperties + 1)},
		want: &B0{B: makeUint8Slice(maxIndexedProperties + 1)},
	},
	{
		desc:    "long []int8 is too long",
		src:     &B1{B: makeInt8Slice(maxIndexedProperties + 1)},
		want:    &B1{},
		saveErr: "gae: too many indexed properties",
	},
	{
		desc: "short []int8",
		src:  &B1{B: makeInt8Slice(3)},
		want: &B1{B: makeInt8Slice(3)},
	},
	{
		desc: "long myBlob",
		src:  &B2{B: makeUint8Slice(maxIndexedProperties + 1)},
		want: &B2{B: makeUint8Slice(maxIndexedProperties + 1)},
	},
	{
		desc: "short myBlob",
		src:  &B2{B: makeUint8Slice(3)},
		want: &B2{B: makeUint8Slice(3)},
	},
	{
		desc: "long []myByte",
		src:  &B3{B: makeMyByteSlice(maxIndexedProperties + 1)},
		want: &B3{B: makeMyByteSlice(maxIndexedProperties + 1)},
	},
	{
		desc: "short []myByte",
		src:  &B3{B: makeMyByteSlice(3)},
		want: &B3{B: makeMyByteSlice(3)},
	},
	{
		desc: "slice of blobs",
		src: &B4{B: [][]byte{
			makeUint8Slice(3),
			makeUint8Slice(4),
			makeUint8Slice(5),
		}},
		want: &B4{B: [][]byte{
			makeUint8Slice(3),
			makeUint8Slice(4),
			makeUint8Slice(5),
		}},
	},
	{
		desc: "short gae.DSByteString",
		src:  &B5{B: gae.DSByteString(makeUint8Slice(3))},
		want: &B5{B: gae.DSByteString(makeUint8Slice(3))},
	},
	{
		desc: "short gae.DSByteString as props",
		src:  &B5{B: gae.DSByteString(makeUint8Slice(3))},
		want: &gae.DSPropertyMap{
			"B": {mp(gae.DSByteString(makeUint8Slice(3)))},
		},
	},
	{
		desc: "[]byte must be noindex",
		src: &gae.DSPropertyMap{
			"B": {mp(makeUint8Slice(3))},
		},
		actualNoIndex: true,
	},
	{
		desc: "save tagged load props",
		src:  &Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		want: &gae.DSPropertyMap{
			// A and B are renamed to a and b; A and C are noindex, I is ignored.
			// Indexed properties are loaded before raw properties. Thus, the
			// result is: b, b, b, D, E, a, c.
			"b1": {
				mp(21),
				mp(22),
				mp(23),
			},
			"D": {mp(4)},
			"E": {mp(5)},
			"a": {mp(1, true)},
			"C": {mp(3, true)},
			"J": {mp(7, true)},
		},
	},
	{
		desc: "save tagged load tagged",
		src:  &Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		want: &Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, J: 7},
	},
	{
		desc: "save props load tagged",
		src: &gae.DSPropertyMap{
			"A": {mp(11, true)},
			"a": {mp(12, true)},
		},
		want:    &Tagged{A: 12},
		convErr: `cannot load field "A"`,
	},
	{
		desc:   "invalid tagged1",
		src:    &InvalidTagged1{I: 1},
		plsErr: `struct tag has invalid property name: "\t"`,
	},
	{
		desc:   "invalid tagged2",
		src:    &InvalidTagged2{I: 1, J: 2},
		want:   &InvalidTagged2{},
		plsErr: `struct tag has repeated property name: "I"`,
	},
	{
		desc:   "invalid tagged3",
		src:    &InvalidTagged3{I: 1},
		plsErr: `struct tag has invalid property name: "a\t"`,
	},
	{
		desc:   "invalid tagged4",
		src:    &InvalidTagged4{I: 1},
		plsErr: `struct tag has invalid property name: "a."`,
	},
	{
		desc:   "invalid tagged5",
		src:    &InvalidTagged5{I: 19, V: []InvalidTaggedSub{{1}}},
		plsErr: `struct tag has repeated property name: "V.I"`,
	},
	{
		desc: "doubler",
		src:  &Doubler{S: "s", I: 1, B: true},
		want: &Doubler{S: "ss", I: 2, B: true},
	},
	{
		desc: "save struct load props",
		src:  &X0{S: "s", I: 1},
		want: &gae.DSPropertyMap{
			"S": {mp("s")},
			"I": {mp(1)},
		},
	},
	{
		desc: "save props load struct",
		src: &gae.DSPropertyMap{
			"S": {mp("s")},
			"I": {mp(1)},
		},
		want: &X0{S: "s", I: 1},
	},
	{
		desc: "nil-value props",
		src: &gae.DSPropertyMap{
			"I": {mp(nil)},
			"B": {mp(nil)},
			"S": {mp(nil)},
			"F": {mp(nil)},
			"K": {mp(nil)},
			"T": {mp(nil)},
			"J": {
				mp(nil),
				mp(7),
				mp(nil),
			},
		},
		want: &struct {
			I int64
			B bool
			S string
			F float64
			K gae.DSKey
			T time.Time
			J []int64
		}{
			J: []int64{0, 7, 0},
		},
	},
	{
		desc: "save outer load props",
		src: &Outer{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
		},
		want: &gae.DSPropertyMap{
			"A": {mp(1)},
			"I.W": {
				mp(10),
				mp(20),
				mp(30),
			},
			"I.X": {
				mp("ten"),
				mp("twenty"),
				mp("thirty"),
			},
			"J.Y": {mp(3.14)},
			"Z":   {mp(true)},
		},
	},
	{
		desc: "save props load outer-equivalent",
		src: &gae.DSPropertyMap{
			"A": {mp(1)},
			"I.W": {
				mp(10),
				mp(20),
				mp(30),
			},
			"I.X": {
				mp("ten"),
				mp("twenty"),
				mp("thirty"),
			},
			"J.Y": {mp(3.14)},
			"Z":   {mp(true)},
		},
		want: &OuterEquivalent{
			A:     1,
			IDotW: []int32{10, 20, 30},
			IDotX: []string{"ten", "twenty", "thirty"},
			JDotY: 3.14,
			Z:     true,
		},
	},
	{
		desc: "save outer-equivalent load outer",
		src: &OuterEquivalent{
			A:     1,
			IDotW: []int32{10, 20, 30},
			IDotX: []string{"ten", "twenty", "thirty"},
			JDotY: 3.14,
			Z:     true,
		},
		want: &Outer{
			A: 1,
			I: []Inner1{
				{10, "ten"},
				{20, "twenty"},
				{30, "thirty"},
			},
			J: Inner2{
				Y: 3.14,
			},
			Inner3: Inner3{
				Z: true,
			},
		},
	},
	{
		desc: "dotted names save",
		src:  &Dotted{A: DottedA{B: DottedB{C: 88}}},
		want: &gae.DSPropertyMap{
			"A0.A1.A2.B3.C4.C5": {mp(88)},
		},
	},
	{
		desc: "dotted names load",
		src: &gae.DSPropertyMap{
			"A0.A1.A2.B3.C4.C5": {mp(99)},
		},
		want: &Dotted{A: DottedA{B: DottedB{C: 99}}},
	},
	{
		desc: "save struct load deriver",
		src:  &X0{S: "s", I: 1},
		want: &Deriver{S: "s", Derived: "derived+s"},
	},
	{
		desc: "save deriver load struct",
		src:  &Deriver{S: "s", Derived: "derived+s", Ignored: "ignored"},
		want: &X0{S: "s"},
	},
	// Regression: CL 25062824 broke handling of appengine.BlobKey fields.
	{
		desc: "appengine.BlobKey",
		src:  &BK{Key: "blah"},
		want: &BK{Key: "blah"},
	},
	{
		desc: "zero time.Time",
		src:  &T{T: time.Time{}},
		want: &T{T: time.Time{}},
	},
	{
		desc: "time.Time near Unix zero time",
		src:  &T{T: time.Unix(0, 4e3).UTC()},
		want: &T{T: time.Unix(0, 4e3).UTC()},
	},
	{
		desc: "time.Time, far in the future",
		src:  &T{T: time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC)},
		want: &T{T: time.Date(99999, 1, 1, 0, 0, 0, 0, time.UTC)},
	},
	{
		desc:    "time.Time, very far in the past",
		src:     &T{T: time.Date(-300000, 1, 1, 0, 0, 0, 0, time.UTC)},
		want:    &T{},
		saveErr: "time value out of range",
	},
	{
		desc:    "time.Time, very far in the future",
		src:     &T{T: time.Date(294248, 1, 1, 0, 0, 0, 0, time.UTC)},
		want:    &T{},
		saveErr: "time value out of range",
	},
	{
		desc: "structs",
		src: &N0{
			X0:       X0{S: "one", I: 2, i: 3},
			Nonymous: X0{S: "four", I: 5, i: 6},
			Ignore:   "ignore",
			Other:    "other",
		},
		want: &N0{
			X0:       X0{S: "one", I: 2},
			Nonymous: X0{S: "four", I: 5},
			Other:    "other",
		},
	},
	{
		desc: "exotic types",
		src: &ExoticTypes{
			BS:   "sup",
			DSBS: gae.DSByteString("nerds"),
		},
		want: &ExoticTypes{
			BS:   "sup",
			DSBS: gae.DSByteString("nerds"),
		},
	},
	{
		desc:   "underspecified types",
		src:    &Underspecified{},
		plsErr: "non-concrete interface",
	},
	{
		desc: "mismatch (string)",
		src: &gae.DSPropertyMap{
			"K": {mp(199)},
			"S": {mp([]byte("cats"))},
			"F": {mp(gae.DSByteString("nurbs"))},
		},
		want:    &MismatchTypes{},
		convErr: "type mismatch",
	},
	{
		desc:    "mismatch (float)",
		src:     &gae.DSPropertyMap{"F": {mp(gae.BSKey("wot"))}},
		want:    &MismatchTypes{},
		convErr: "type mismatch",
	},
	{
		desc:    "mismatch (float/overflow)",
		src:     &gae.DSPropertyMap{"F": {mp(math.MaxFloat64)}},
		want:    &MismatchTypes{},
		convErr: "overflows",
	},
	{
		desc:    "mismatch (key)",
		src:     &gae.DSPropertyMap{"K": {mp(false)}},
		want:    &MismatchTypes{},
		convErr: "type mismatch",
	},
	{
		desc:    "mismatch (bool)",
		src:     &gae.DSPropertyMap{"B": {mp(testKey0)}},
		want:    &MismatchTypes{},
		convErr: "type mismatch",
	},
	{
		desc:    "mismatch (time)",
		src:     &gae.DSPropertyMap{"T": {mp(gae.DSGeoPoint{})}},
		want:    &MismatchTypes{},
		convErr: "type mismatch",
	},
	{
		desc:    "mismatch (geopoint)",
		src:     &gae.DSPropertyMap{"G": {mp(time.Now().UTC())}},
		want:    &MismatchTypes{},
		convErr: "type mismatch",
	},
	{
		desc: "slice of structs",
		src: &N1{
			X0: X0{S: "one", I: 2, i: 3},
			Nonymous: []X0{
				{S: "four", I: 5, i: 6},
				{S: "seven", I: 8, i: 9},
				{S: "ten", I: 11, i: 12},
				{S: "thirteen", I: 14, i: 15},
			},
			Ignore: "ignore",
			Other:  "other",
		},
		want: &N1{
			X0: X0{S: "one", I: 2},
			Nonymous: []X0{
				{S: "four", I: 5},
				{S: "seven", I: 8},
				{S: "ten", I: 11},
				{S: "thirteen", I: 14},
			},
			Other: "other",
		},
	},
	{
		desc: "structs with slices of structs",
		src: &N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		want: &N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
	},
	{
		desc: "save structs load props",
		src: &N2{
			N1: N1{
				X0: X0{S: "rouge"},
				Nonymous: []X0{
					{S: "rosso0"},
					{S: "rosso1"},
				},
			},
			Green: N1{
				X0: X0{S: "vert"},
				Nonymous: []X0{
					{S: "verde0"},
					{S: "verde1"},
					{S: "verde2"},
				},
			},
			Blue: N1{
				X0: X0{S: "bleu"},
				Nonymous: []X0{
					{S: "blu0"},
					{S: "blu1"},
					{S: "blu2"},
					{S: "blu3"},
				},
			},
		},
		want: &gae.DSPropertyMap{
			"red.S":            {mp("rouge")},
			"red.I":            {mp(0)},
			"red.Nonymous.S":   {mp("rosso0"), mp("rosso1")},
			"red.Nonymous.I":   {mp(0), mp(0)},
			"red.Other":        {mp("")},
			"green.S":          {mp("vert")},
			"green.I":          {mp(0)},
			"green.Nonymous.S": {mp("verde0"), mp("verde1"), mp("verde2")},
			"green.Nonymous.I": {mp(0), mp(0), mp(0)},
			"green.Other":      {mp("")},
			"Blue.S":           {mp("bleu")},
			"Blue.I":           {mp(0)},
			"Blue.Nonymous.S":  {mp("blu0"), mp("blu1"), mp("blu2"), mp("blu3")},
			"Blue.Nonymous.I":  {mp(0), mp(0), mp(0), mp(0)},
			"Blue.Other":       {mp("")},
		},
	},
	{
		desc: "save props load structs with ragged fields",
		src: &gae.DSPropertyMap{
			"red.S":            {mp("rot")},
			"green.Nonymous.I": {mp(10), mp(11), mp(12), mp(13)},
			"Blue.Nonymous.S":  {mp("blau0"), mp("blau1"), mp("blau2")},
			"Blue.Nonymous.I":  {mp(20), mp(21)},
		},
		want: &N2{
			N1: N1{
				X0: X0{S: "rot"},
			},
			Green: N1{
				Nonymous: []X0{
					{I: 10},
					{I: 11},
					{I: 12},
					{I: 13},
				},
			},
			Blue: N1{
				Nonymous: []X0{
					{S: "blau0", I: 20},
					{S: "blau1", I: 21},
					{S: "blau2"},
				},
			},
		},
	},
	{
		desc: "save structs with noindex tags",
		src: &struct {
			A struct {
				X string `gae:",noindex"`
				Y string
			} `gae:",noindex"`
			B struct {
				X string `gae:",noindex"`
				Y string
			}
		}{},
		want: &gae.DSPropertyMap{
			"B.Y": {mp("")},
			"A.X": {mp("", true)},
			"A.Y": {mp("", true)},
			"B.X": {mp("", true)},
		},
	},
	{
		desc: "embedded struct with name override",
		src: &struct {
			Inner1 `gae:"foo"`
		}{},
		want: &gae.DSPropertyMap{
			"foo.W": {mp(0)},
			"foo.X": {mp("")},
		},
	},
	{
		desc:   "slice of slices",
		src:    &SliceOfSlices{},
		plsErr: `flattening nested structs leads to a slice of slices: field "S"`,
	},
	{
		desc:   "recursive struct",
		src:    &Recursive{},
		plsErr: `field "R" is recursively defined`,
	},
	{
		desc:   "mutually recursive struct",
		src:    &MutuallyRecursive0{},
		plsErr: `field "R" has problem: field "R" is recursively defined`,
	},
	{
		desc: "non-exported struct fields",
		src: &struct {
			i, J int64
		}{i: 1, J: 2},
		want: &gae.DSPropertyMap{
			"J": {mp(2)},
		},
	},
	{
		desc: "json.RawMessage",
		src: &struct {
			J json.RawMessage
		}{
			J: json.RawMessage("rawr"),
		},
		want: &gae.DSPropertyMap{
			"J": {mp([]byte("rawr"))},
		},
	},
	{
		desc: "json.RawMessage to myBlob",
		src: &struct {
			B json.RawMessage
		}{
			B: json.RawMessage("rawr"),
		},
		want: &B2{B: myBlob("rawr")},
	},
}

// checkErr returns the empty string if either both want and err are zero,
// or if want is a non-empty substring of err's string representation.
func checkErr(want string, err error) string {
	if err != nil {
		got := err.Error()
		if want == "" || strings.Index(got, want) == -1 {
			return got
		}
	} else if want != "" {
		return fmt.Sprintf("want error %q", want)
	}
	return ""
}

func ShouldErrLike(actual interface{}, expected ...interface{}) string {
	e2s := func(o interface{}) (string, bool) {
		switch x := o.(type) {
		case nil:
			return "", true
		case string:
			return x, true
		case error:
			if x != nil {
				return x.Error(), true
			}
			return "", true
		}
		return fmt.Sprintf("unknown argument type %T, expected string, error or nil", actual), false
	}

	as, ok := e2s(actual)
	if !ok {
		return as
	}

	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, got %d", len(expected))
	}

	err, ok := e2s(expected[0])
	if !ok {
		return err
	}
	return ShouldContainSubstring(as, err)
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	checkErr := func(actual interface{}, expected string) bool {
		So(actual, ShouldErrLike, expected)
		return expected != ""
	}

	Convey("Test round-trip", t, func() {
		for _, tc := range testCases {
			tc := tc
			Convey(tc.desc, func() {
				pls, err := GetPLS(tc.src)
				if checkErr(err, tc.plsErr) {
					return
				}
				So(pls, ShouldNotBeNil)

				savedProps, err := pls.Save()
				if checkErr(err, tc.saveErr) {
					return
				}
				So(savedProps, ShouldNotBeNil)

				if tc.actualNoIndex {
					for _, props := range savedProps {
						So(props[0].NoIndex(), ShouldBeTrue)
						return
					}
					So(true, ShouldBeFalse) // shouldn't get here
				}

				var got interface{}
				if _, ok := tc.want.(*gae.DSPropertyMap); ok {
					got = &gae.DSPropertyMap{}
				} else {
					got = reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
				}

				pls, err = GetPLS(got)
				if checkErr(err, tc.plsLoadErr) {
					return
				}
				So(pls, ShouldNotBeNil)

				convErrs, err := pls.Load(savedProps)
				if checkErr(err, tc.loadErr) {
					return
				}
				if len(tc.convErr) == 0 {
					So(convErrs, ShouldBeNil)
				} else {
					So(convErrs[0], ShouldContainSubstring, tc.convErr)
				}
				if tc.want == nil {
					return
				}

				if gotT, ok := got.(*T); ok {
					// Round tripping a time.Time can result in a different time.Location: Local instead of UTC.
					// We therefore test equality explicitly, instead of relying on reflect.DeepEqual.
					So(gotT.T.Equal(tc.want.(*T).T), ShouldBeTrue)
				} else {
					So(got, ShouldResemble, tc.want)
				}
			})
		}
	})
}

func TestSpecial(t *testing.T) {
	t.Parallel()

	Convey("Test special fields", t, func() {
		Convey("Can retrieve from struct", func() {
			o := &N0{ID: 100}
			pls := GetStructPLS(o)
			val, current, err := pls.GetSpecial("id")
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "")
			So(current, ShouldEqual, 100)

			val, current, err = pls.GetSpecial("kind")
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "whatnow")
			So(current, ShouldEqual, nil)
		})

		Convey("Getting something not there is an error", func() {
			o := &N0{ID: 100}
			pls := GetStructPLS(o)
			_, _, err := pls.GetSpecial("wat")
			So(err, ShouldEqual, gae.ErrDSSpecialFieldUnset)
		})

		Convey("getting/setting from a bad struct is an error", func() {
			o := &Recursive{}
			pls := GetStructPLS(o)
			_, _, err := pls.GetSpecial("wat")
			So(err, ShouldNotBeNil)

			err = pls.SetSpecial("wat", 100)
			So(err, ShouldNotBeNil)
		})

		Convey("can assign values to exported special fields", func() {
			o := &N0{ID: 100}
			pls := GetStructPLS(o)
			err := pls.SetSpecial("id", int64(200))
			So(err, ShouldBeNil)
			So(o.ID, ShouldEqual, 200)

		})

		Convey("assigning to unsassiagnable fields is a simple error", func() {
			o := &N0{ID: 100}
			pls := GetStructPLS(o)
			err := pls.SetSpecial("kind", "hi")
			So(err.Error(), ShouldContainSubstring, "unexported field")

			err = pls.SetSpecial("noob", "hi")
			So(err, ShouldEqual, gae.ErrDSSpecialFieldUnset)
		})
	})

	Convey("StructPLS Miscellaneous", t, func() {
		Convey("multiple overlapping fields is an error", func() {
			o := &BadSpecial{}
			pls := GetStructPLS(o)
			convErr, err := pls.Load(nil)
			So(convErr, ShouldBeNil)
			So(err, ShouldErrLike, "multiple times")
			e := pls.Problem()
			_, err = pls.Save()
			So(err, ShouldEqual, e)
			_, err = pls.Load(nil)
			So(err, ShouldEqual, e)
		})

		Convey("can transform a list of things into a list of PLSs", func() {
			o := []interface{}{
				&N0{X0: X0{S: "hi", I: 5}},
				&N0{Nonymous: X0{S: "hi", I: 5}},
				&gae.DSPropertyMap{
					"Nerd": {mp(10), mp(false)},
					"What": {mp("is"), mp("up")},
				},
			}
			plss, err := MultiGetPLS(o)
			So(err, ShouldBeNil)
			for i, pls := range plss {
				pmap, err := pls.Save()
				targ := gae.DSPropertyLoadSaver(&gae.DSPropertyMap{})
				obj := interface{}(targ)
				if i < 2 {
					obj = &N0{}
					targ = GetStructPLS(obj)
				}
				convErr, err := targ.Load(pmap)
				So(err, ShouldBeNil)
				So(convErr, ShouldBeNil)
				So(obj, ShouldResemble, o[i])
			}
		})

		Convey("list of DSPropertyLoadSavers is a shortcut", func() {
			o := []gae.DSPropertyLoadSaver{&gae.DSPropertyMap{}}
			plss, err := MultiGetPLS(o)
			So(err, ShouldBeNil)
			So(&plss[0], ShouldEqual, &o[0]) // identical underlying array
		})

		Convey("attempting to transform a bad object is an error", func() {
			o := []int{100}
			_, err := MultiGetPLS(o)
			So(err, ShouldEqual, gae.ErrDSInvalidEntityType)

			f := false
			_, err = MultiGetPLS(&f)
			So(err, ShouldErrLike, "bad type")
		})

		Convey("empty property names are invalid", func() {
			So(validPropertyName(""), ShouldBeFalse)
		})
	})
}
