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

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/blobstore"
	pb "go.chromium.org/luci/gae/service/datastore/internal/protos/datastore"
	"go.chromium.org/luci/gae/service/datastore/internal/testprotos"
)

var (
	mp   = MkProperty
	mpNI = MkPropertyNI
)

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
	testKey0     = mkKey("kind", "name0")
	testKey1a    = mkKey("kind", "name1")
	testKey1b    = mkKey("kind", "name1")
	testKey2a    = mkKey("kind", "name0", "kind", "name2")
	testKey2b    = mkKey("kind", "name0", "kind", "name2")
	testGeoPt0   = GeoPoint{Lat: 1.2, Lng: 3.4}
	testGeoPt1   = GeoPoint{Lat: 5, Lng: 10}
	testBadGeoPt = GeoPoint{Lat: 1000, Lng: 34}
)

type B0 struct {
	B []byte `gae:",noindex"`
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
	B [][]byte `gae:",noindex"`
}

type B5 struct {
	B []byte
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
	G GeoPoint
}

type G1 struct {
	G []GeoPoint
}

type K0 struct {
	K *Key
}

type K1 struct {
	K []*Key
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

type N3 struct {
	ID uint32 `gae:"$id,200"`
}

type O0 struct {
	I int64
}

type O1 struct {
	I int32
}

type U0 struct {
	U uint32
}

type U1 struct {
	U byte
}

type U2 struct {
	U int64
}

type T struct {
	T time.Time
}

type TSlice struct {
	TSlice []time.Time
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

type InvalidTagged6 struct {
	I int `json:"f" gae:f,noindex`
}

type Unexported struct {
	private string `gae:"Private"`
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

type DeeperSlice struct {
	A []DeeperA
}

type DeeperA struct {
	B DeeperB
}

type DeeperB struct {
	C string
}

type LSPOuter struct {
	One   LSPInner   `gae:",lsp"`
	Slice []LSPInner `gae:",lsp"`
}

type LSPOuterUnindexed struct {
	One   LSPInner   `gae:",lsp,noindex"`
	Slice []LSPInner `gae:",lsp,noindex"`
}

type LSPInner struct {
	ID          string `gae:"$id"`
	Kind        string `gae:"$kind"`
	Ints        []int64
	IntsNoIndex []int64     `gae:",noindex"`
	Deeper1     LSPDeeper   `gae:",lsp"`
	Deeper2     []LSPDeeper `gae:",lsp"`
	Bool        bool
	Blob        []byte
	String      string
	Double      float64
}

type LSPDeeper struct {
	ID    int64 `gae:"$id"`
	Field int64
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
	BS blobstore.Key
}

type Underspecified struct {
	Iface PropertyConverter
}

type MismatchTypes struct {
	S  string
	B  bool
	F  float32
	K  *Key
	T  time.Time
	G  GeoPoint
	IS []int
}

type BadMeta struct {
	ID int64  `gae:"$id"`
	id string `gae:"$id"`
}

type Doubler struct {
	S string
	I int64
	B bool

	t testing.TB
}

func (d *Doubler) Load(props PropertyMap) error {
	return GetPLS(d).Load(props)
}

func (d *Doubler) Save(withMeta bool) (PropertyMap, error) {
	pls := GetPLS(d)
	propMap, err := pls.Save(withMeta)
	if err != nil {
		return nil, err
	}

	double := func(prop *Property) {
		switch v := prop.Value().(type) {
		case string:
			// + means string concatenation.
			assert.Loosely(d.t, prop.SetValue(v+v, prop.IndexSetting()), should.BeNil)
		case int64:
			// + means integer addition.
			assert.Loosely(d.t, prop.SetValue(v+v, prop.IndexSetting()), should.BeNil)
		}
	}

	// Edit that map and send it on.
	for k, v := range propMap {
		if prop, ok := v.(Property); ok {
			double(&prop)
			propMap[k] = prop
		}
	}
	return propMap, nil
}

func (d *Doubler) Problem() error { return nil }

var _ PropertyLoadSaver = (*Doubler)(nil)

type Deriver struct {
	S, Derived, Ignored string
}

func (d *Deriver) Load(props PropertyMap) error {
	for name := range props {
		if name != "S" {
			continue
		}
		d.S = props.Slice(name)[0].Value().(string)
		d.Derived = "derived+" + d.S
	}
	return nil
}

func (d *Deriver) Save(withMeta bool) (PropertyMap, error) {
	return map[string]PropertyData{
		"S": mp(d.S),
	}, nil
}

func (d *Deriver) Problem() error { return nil }

var _ PropertyLoadSaver = (*Deriver)(nil)

type Augmenter struct {
	S string

	g string `gae:"-"`
}

func (a *Augmenter) Load(props PropertyMap) error {
	if e := props.Slice("Extra"); len(e) > 0 {
		a.g = e[0].Value().(string)
		delete(props, "Extra")
	}
	if err := GetPLS(a).Load(props); err != nil {
		return err
	}
	return nil
}

func (a *Augmenter) Save(withMeta bool) (PropertyMap, error) {
	props, err := GetPLS(a).Save(withMeta)
	if err != nil {
		return nil, err
	}
	props["Extra"] = MkProperty("ohai!")
	return props, nil
}

func (a *Augmenter) Problem() error { return nil }

var _ PropertyLoadSaver = (*Augmenter)(nil)

type BK struct {
	Key blobstore.Key
}

type Convertable []int64

var _ PropertyConverter = (*Convertable)(nil)

func (c *Convertable) ToProperty() (ret Property, err error) {
	buf := make([]string, len(*c))
	for i, v := range *c {
		buf[i] = strconv.FormatInt(v, 10)
	}
	err = ret.SetValue(strings.Join(buf, ","), NoIndex)
	return
}

func (c *Convertable) FromProperty(pv Property) error {
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

func (c *Convertable2) ToProperty() (ret Property, err error) {
	err = ret.SetValue(c.Data, ShouldIndex)
	return
}

func (c *Convertable2) FromProperty(pv Property) error {
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

type JSONKVProp map[string]any

var _ PropertyConverter = (*JSONKVProp)(nil)

func (j *JSONKVProp) ToProperty() (ret Property, err error) {
	data, err := json.Marshal(map[string]any(*j))
	if err != nil {
		return
	}
	err = ret.SetValue(data, NoIndex)
	return
}

func (j *JSONKVProp) FromProperty(pv Property) error {
	if bval, ok := pv.Value().([]byte); ok {
		dec := json.NewDecoder(bytes.NewBuffer(bval))
		dec.UseNumber()
		return dec.Decode((*map[string]any)(j))
	}
	return fmt.Errorf("nope")
}

type Impossible3 struct {
	KMap JSONKVProp `gae:"kewelmap"`
}

type Complex complex128

var _ PropertyConverter = (*Complex)(nil)

func (c *Complex) ToProperty() (ret Property, err error) {
	// cheat hardkore and usurp GeoPoint so datastore will index these suckers
	// (note that this won't REALLY work, since GeoPoints are limited to a very
	// limited range of values, but it's nice to pretend ;)). You'd probably
	// really end up with a packed binary representation.
	err = ret.SetValue(GeoPoint{Lat: real(*c), Lng: imag(*c)}, ShouldIndex)
	return
}

func (c *Complex) FromProperty(p Property) error {
	if gval, ok := p.Value().(GeoPoint); ok {
		*c = Complex(complex(gval.Lat, gval.Lng))
		return nil
	}
	return fmt.Errorf("nope")
}

type Impossible4 struct {
	Values []Complex
}

type DerivedKey struct {
	K *Key
}

type IfaceKey struct {
	K *Key
}

type IDParser struct {
	_kind string `gae:"$kind,CoolKind"`

	// real $id is myParentName|myID
	parent string `gae:"-"`
	id     int64  `gae:"-"`
}

var _ MetaGetterSetter = (*IDParser)(nil)

func (i *IDParser) getFullID() string {
	return fmt.Sprintf("%s|%d", i.parent, i.id)
}

func (i *IDParser) GetAllMeta() PropertyMap {
	pm := GetPLS(i).GetAllMeta()
	pm.SetMeta("id", i.getFullID())
	return pm
}

func (i *IDParser) GetMeta(key string) (any, bool) {
	if key == "id" {
		return i.getFullID(), true
	}
	return GetPLS(i).GetMeta(key)
}

func (i *IDParser) SetMeta(key string, value any) bool {
	if key == "id" {
		// let the panics flooowwww
		vS := strings.SplitN(value.(string), "|", 2)
		i.parent = vS[0]
		var err error
		i.id, err = strconv.ParseInt(vS[1], 10, 64)
		if err != nil {
			panic(err)
		}
		return true
	}
	return GetPLS(i).SetMeta(key, value)
}

type KindOverride struct {
	ID int64 `gae:"$id"`

	customKind string `gae:"-"`
}

var _ MetaGetterSetter = (*KindOverride)(nil)

func (i *KindOverride) GetAllMeta() PropertyMap {
	pm := GetPLS(i).GetAllMeta()
	if i.customKind != "" {
		pm.SetMeta("kind", i.customKind)
	}
	return pm
}

func (i *KindOverride) GetMeta(key string) (any, bool) {
	if key == "kind" && i.customKind != "" {
		return i.customKind, true
	}
	return GetPLS(i).GetMeta(key)
}

func (i *KindOverride) SetMeta(key string, value any) bool {
	if key == "kind" {
		kind := value.(string)
		if kind != "KindOverride" {
			i.customKind = kind
		} else {
			i.customKind = ""
		}
		return true
	}
	return GetPLS(i).SetMeta(key, value)
}

type EmbeddedID struct {
	Thing string
	Val   int
}

var _ PropertyConverter = (*EmbeddedID)(nil)

func (e *EmbeddedID) ToProperty() (ret Property, err error) {
	return mpNI(fmt.Sprintf("%s|%d", e.Thing, e.Val)), nil
}

func (e *EmbeddedID) FromProperty(val Property) error {
	if val.Type() != PTString {
		return fmt.Errorf("gotta have a string")
	}
	toks := strings.SplitN(val.Value().(string), "|", 2)
	if len(toks) != 2 {
		return fmt.Errorf("gotta have two parts")
	}
	v, err := strconv.Atoi(toks[1])
	if err != nil {
		return err
	}

	e.Thing = toks[0]
	e.Val = v
	return nil
}

type IDEmbedder struct {
	EmbeddedID `gae:"$id"`
}

type Simple struct{}

type SubStructForReset struct {
	SubSlice   []string
	subPrivate int
}

type StructForReset struct {
	ID     string      `gae:"$id"`
	Parent *Key        `gae:"$parent"`
	Extra  PropertyMap `gae:",extra"`
	Field  string      `gae:"NoIndexIsAlsoReset,noindex"`
	Struct SubStructForReset
	Slice  []string
	Opt    Optional[int64, Indexed]

	Skip          string `gae:"-"`
	private       int
	privateStruct SubStructForReset
	privateSlice  []int
}

// protoEmbedder should be implemented by all proto-containing structs to
// specific comparison in TestRoundTrip.
type protoEmbedder interface {
	embedsProtos()
}

type WithProtoDefault struct {
	Kind string `gae:"$kind,WithProto"`

	Msg *testprotos.Msg
}

type WithProtoCompress struct {
	Kind string `gae:"$kind,WithProto"`

	Msg *testprotos.Msg `gae:",zstd"`
}

type WithProtoLegacy struct {
	Kind string `gae:"$kind,WithProto"`

	Msg *testprotos.Msg `gae:",legacy"`
}

type WithProtoOmitted struct {
	Omitted *testprotos.Msg `gae:"-"`
}

type WithProtoMeta struct {
	Msg *testprotos.Msg `gae:"$what"`
}

type WithProtoIndirect struct {
	Inner WithProtoDefault
}
type WithProtoIndirectSlice struct {
	Inner []WithProtoDefault
}

type WithAnonymousProto struct {
	*testprotos.Msg
}

func (*WithProtoDefault) embedsProtos()       {}
func (*WithProtoCompress) embedsProtos()      {}
func (*WithProtoLegacy) embedsProtos()        {}
func (*WithProtoOmitted) embedsProtos()       {}
func (*WithProtoMeta) embedsProtos()          {}
func (*WithProtoIndirect) embedsProtos()      {}
func (*WithProtoIndirectSlice) embedsProtos() {}
func (*WithAnonymousProto) embedsProtos()     {}

type IntAlias int64

type Optionals struct {
	Int       Optional[int64, Indexed]
	IntAlias  Optional[IntAlias, Indexed]
	Unindexed Optional[int64, Unindexed]
	String    Optional[string, Indexed]
	Bytes     Optional[[]byte, Indexed]
	Time      Optional[time.Time, Indexed]
	Geo       Optional[GeoPoint, Indexed]
	Key       Optional[*Key, Indexed]
	Slice     []Optional[int64, Indexed]
}

type Nullables struct {
	Indexed   Nullable[int64, Indexed]
	Unindexed Nullable[int64, Unindexed]
}

func legacyLSP(props ...*pb.Property) []byte {
	blob, err := (proto.MarshalOptions{AllowPartial: true}).Marshal(&pb.EntityProto{
		Property: props,
	})
	if err != nil {
		panic(err)
	}
	return blob
}

func legacyProp(name string, val any) *pb.Property {
	switch v := val.(type) {
	case int64:
		return &pb.Property{Name: &name, Value: &pb.PropertyValue{
			Int64Value: &v,
		}}
	case bool:
		return &pb.Property{Name: &name, Value: &pb.PropertyValue{
			BooleanValue: &v,
		}}
	case string:
		return &pb.Property{Name: &name, Value: &pb.PropertyValue{
			StringValue: &v,
		}}
	case []byte:
		str := string(v)
		meaning := pb.Property_BYTESTRING
		return &pb.Property{Name: &name, Meaning: &meaning, Value: &pb.PropertyValue{
			StringValue: &str,
		}}
	case float64:
		return &pb.Property{Name: &name, Value: &pb.PropertyValue{
			DoubleValue: &v,
		}}
	default:
		panic(fmt.Sprintf("unexpected type %T", val))
	}
}

func deflate(blob []byte) []byte {
	out := bytes.NewBuffer(nil)
	w := zlib.NewWriter(out)
	if _, err := w.Write(blob); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return out.Bytes()
}

type testCase struct {
	desc       string
	src        any
	loadInto   any // if set, used for loading instead a newly created object.
	want       any
	plsErr     string
	saveErr    string
	plsLoadErr string
	loadErr    string
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
		want: PropertyMap{
			"G": mp(testGeoPt0),
		},
	},
	{
		desc: "geopoint slice",
		src:  &G1{G: []GeoPoint{testGeoPt0, testGeoPt1}},
		want: &G1{G: []GeoPoint{testGeoPt0, testGeoPt1}},
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
		src:  &K1{[]*Key{nil, nil}},
		want: &K1{[]*Key{nil, nil}},
	},
	{
		desc: "some nil keys in slice",
		src:  &K1{[]*Key{testKey1a, nil, testKey2a}},
		want: &K1{[]*Key{testKey1b, nil, testKey2b}},
	},
	{
		desc:    "overflow",
		src:     &O0{I: 1 << 48},
		want:    &O1{},
		loadErr: "overflow",
	},
	{
		desc:    "underflow",
		src:     &O0{I: math.MaxInt64},
		want:    &O1{},
		loadErr: "overflow",
	},
	{
		desc: "time",
		src:  &T{T: time.Unix(1e9, 0).UTC()},
		want: &T{T: time.Unix(1e9, 0).UTC()},
	},
	{
		desc: "time as props",
		src:  &T{T: time.Unix(1e9, 0).UTC()},
		want: PropertyMap{
			"T": mp(time.Unix(1e9, 0).UTC()),
		},
	},
	{
		desc: "time as props rounds to microseconds",
		src:  &T{T: time.Unix(1e9, 111333555).UTC()},
		want: &T{T: time.Unix(1e9, 111334000).UTC()},
	},
	{
		desc: "time slice",
		src: &TSlice{TSlice: []time.Time{
			time.Unix(1e9, 0).UTC(),
			time.Unix(2e9, 0).UTC(),
		},
		},
		want: PropertyMap{
			"TSlice": PropertySlice{
				mp(time.Unix(1e9, 0).UTC()),
				mp(time.Unix(2e9, 0).UTC()),
			},
		},
	},
	{
		desc: "uint32 save",
		src:  &U0{U: 1},
		want: PropertyMap{
			"U": mp(1),
		},
	},
	{
		desc: "uint32 load",
		src:  &U2{U: 100},
		want: &U0{U: 100},
	},
	{
		desc:    "uint32 load oob (neg)",
		src:     &U2{U: -1},
		want:    &U0{},
		loadErr: "overflow",
	},
	{
		desc:    "uint32 load oob (huge)",
		src:     &U2{U: math.MaxInt64},
		want:    &U0{},
		loadErr: "overflow",
	},
	{
		desc: "byte save",
		src:  &U1{U: 1},
		want: PropertyMap{
			"U": mp(1),
		},
	},
	{
		desc: "byte load",
		src:  &U2{U: 100},
		want: &U1{U: 100},
	},
	{
		desc:    "byte load oob (neg)",
		src:     &U2{U: -1},
		want:    &U1{},
		loadErr: "overflow",
	},
	{
		desc:    "byte load oob (huge)",
		src:     &U2{U: math.MaxInt64},
		want:    &U1{},
		loadErr: "overflow",
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
		loadErr: "no such struct field",
	},
	{
		desc:    "save string load bool",
		src:     &X0{S: "one", I: 2, i: 3},
		want:    &X3{I: 2},
		loadErr: "type mismatch",
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
		loadErr: "requires a slice",
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
		want: PropertyMap{
			"Nested.wot": PropertySlice{mpNI("1,5,9"), mpNI("2,4,6")},
		},
	},
	{
		desc:    "convertable slice (bad load)",
		src:     PropertyMap{"Nested.wot": mpNI([]byte("ohai"))},
		want:    &Impossible{[]ImpossibleInner{{}}},
		loadErr: "nope",
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
				"no_way!": []any{true, "story"},
				"what":    []any{"is", "really", 100},
			},
		},
		want: &Impossible3{
			JSONKVProp{
				"epic":    "success",
				"no_way!": []any{true, "story"},
				"what":    []any{"is", "really", json.Number("100")},
			},
		},
	},
	{
		desc: "convertable json KVMap (to map)",
		src: &Impossible3{
			JSONKVProp{
				"epic":    "success",
				"no_way!": []any{true, "story"},
				"what":    []any{"is", "really", 100},
			},
		},
		want: PropertyMap{
			"kewelmap": mpNI([]byte(
				`{"epic":"success","no_way!":[true,"story"],"what":["is","really",100]}`)),
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
		want: PropertyMap{
			"Values": PropertySlice{mp(GeoPoint{Lat: 1, Lng: 2}), mp(GeoPoint{Lat: 3, Lng: 4})},
		},
	},
	{
		desc:    "convertable complex slice (bad load)",
		src:     PropertyMap{"Values": mp("hello")},
		want:    &Impossible4{[]Complex(nil)},
		loadErr: "nope",
	},
	{
		desc: "allow concrete *Key implementors (save)",
		src:  &DerivedKey{testKey2a},
		want: &IfaceKey{testKey2b},
	},
	{
		desc: "allow concrete *Key implementors (load)",
		src:  &IfaceKey{testKey2b},
		want: &DerivedKey{testKey2a},
	},
	{
		desc:    "save []float64 load []int64",
		src:     &Y0{B: true, F: []float64{7, 8, 9}},
		want:    &Y2{B: true},
		loadErr: "type mismatch",
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
		desc: "short []byte",
		src:  &B5{B: makeUint8Slice(3)},
		want: &B5{B: makeUint8Slice(3)},
	},
	{
		desc: "short ByteString as props",
		src:  &B5{B: makeUint8Slice(3)},
		want: PropertyMap{
			"B": mp(makeUint8Slice(3)),
		},
	},
	{
		desc: "save tagged load props",
		src:  &Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		want: PropertyMap{
			// A and B are renamed to a and b; A and C are noindex, I is ignored.
			// Indexed properties are loaded before raw properties. Thus, the
			// result is: b, b, b, D, E, a, c.
			"b1": PropertySlice{
				mp(21),
				mp(22),
				mp(23),
			},
			"D": mp(4),
			"E": mp(5),
			"a": mpNI(1),
			"C": mpNI(3),
			"J": mpNI(7),
		},
	},
	{
		desc: "save tagged load tagged",
		src:  &Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, I: 6, J: 7},
		want: &Tagged{A: 1, B: []int{21, 22, 23}, C: 3, D: 4, E: 5, J: 7},
	},
	{
		desc: "save props load tagged",
		src: PropertyMap{
			"A": mpNI(11),
			"a": mpNI(12),
		},
		want:    &Tagged{A: 12},
		loadErr: `cannot load field "A"`,
	},
	{
		desc:   "invalid tagged1",
		src:    &InvalidTagged1{I: 1},
		plsErr: `struct tag for field "I" has invalid property name: "\t"`,
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
		plsErr: `struct tag for field "I" has invalid property name: "a\t"`,
	},
	{
		desc:   "invalid tagged4",
		src:    &InvalidTagged4{I: 1},
		plsErr: `struct tag for field "I" has invalid property name: "a."`,
	},
	{
		desc:   "invalid tagged5",
		src:    &InvalidTagged5{I: 19, V: []InvalidTaggedSub{{1}}},
		plsErr: `struct tag has repeated property name: "V.I"`,
	},
	{
		desc:   "invalid tagged6",
		src:    &InvalidTagged6{I: 19},
		plsErr: `struct tag is invalid: "json:\"f\" gae:f,noindex" (did you mean` + " `gae:\"...\"`?)",
	},
	{
		desc: "doubler",
		src:  &Doubler{S: "s", I: 1, B: true},
		want: &Doubler{S: "ss", I: 2, B: true},
	},
	{
		desc: "save struct load props",
		src:  &X0{S: "s", I: 1},
		want: PropertyMap{
			"S": mp("s"),
			"I": mp(1),
		},
	},
	{
		desc: "save props load struct",
		src: PropertyMap{
			"S": mp("s"),
			"I": mp(1),
		},
		want: &X0{S: "s", I: 1},
	},
	{
		desc: "nil-value props",
		src: PropertyMap{
			"I": mp(nil),
			"B": mp(nil),
			"S": mp(nil),
			"F": mp(nil),
			"K": mp(nil),
			"T": mp(nil),
			"J": PropertySlice{
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
			K *Key
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
		want: PropertyMap{
			"A": mp(1),
			"I.W": PropertySlice{
				mp(10),
				mp(20),
				mp(30),
			},
			"I.X": PropertySlice{
				mp("ten"),
				mp("twenty"),
				mp("thirty"),
			},
			"J.Y": mp(3.14),
			"Z":   mp(true),
		},
	},
	{
		desc: "save props load outer-equivalent",
		src: PropertyMap{
			"A": mp(1),
			"I.W": PropertySlice{
				mp(10),
				mp(20),
				mp(30),
			},
			"I.X": PropertySlice{
				mp("ten"),
				mp("twenty"),
				mp("thirty"),
			},
			"J.Y": mp(3.14),
			"Z":   mp(true),
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
		want: PropertyMap{
			"A0.A1.A2.B3.C4.C5": mp(88),
		},
	},
	{
		desc: "dotted names load",
		src: PropertyMap{
			"A0.A1.A2.B3.C4.C5": mp(99),
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
	{
		desc: "augmenter save",
		src:  &Augmenter{S: "s"},
		want: PropertyMap{
			"S":     mp("s"),
			"Extra": mp("ohai!"),
		},
	},
	{
		desc: "augmenter load",
		src: PropertyMap{
			"S":     mp("s"),
			"Extra": mp("kthxbye!"),
		},
		want: &Augmenter{S: "s", g: "kthxbye!"},
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
			BS: "sup",
		},
		want: &ExoticTypes{
			BS: "sup",
		},
	},
	{
		desc: "exotic type projection",
		src: PropertyMap{
			"BS": mp([]byte("I'mABlobKey")),
		},
		want: &ExoticTypes{
			BS: "I'mABlobKey",
		},
	},
	{
		desc:   "underspecified types",
		src:    &Underspecified{},
		plsErr: "non-concrete interface",
	},
	{
		desc: "mismatch (string)",
		src: PropertyMap{
			"K": mp(199),
			"S": mp([]byte("cats")),
			"F": mp("nurbs"),
		},
		want:    &MismatchTypes{},
		loadErr: "type mismatch",
	},
	{
		desc:    "mismatch (float)",
		src:     PropertyMap{"F": mp(blobstore.Key("wot"))},
		want:    &MismatchTypes{},
		loadErr: "type mismatch",
	},
	{
		desc:    "mismatch (float/overflow)",
		src:     PropertyMap{"F": mp(math.MaxFloat64)},
		want:    &MismatchTypes{},
		loadErr: "overflows",
	},
	{
		desc:    "mismatch (key)",
		src:     PropertyMap{"K": mp(false)},
		want:    &MismatchTypes{},
		loadErr: "type mismatch",
	},
	{
		desc:    "mismatch (bool)",
		src:     PropertyMap{"B": mp(testKey0)},
		want:    &MismatchTypes{},
		loadErr: "type mismatch",
	},
	{
		desc:    "mismatch (time)",
		src:     PropertyMap{"T": mp(GeoPoint{})},
		want:    &MismatchTypes{},
		loadErr: "type mismatch",
	},
	{
		desc:    "mismatch (geopoint)",
		src:     PropertyMap{"G": mp(time.Now().UTC())},
		want:    &MismatchTypes{},
		loadErr: "type mismatch",
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
		want: PropertyMap{
			"red.S":            mp("rouge"),
			"red.I":            mp(0),
			"red.Nonymous.S":   PropertySlice{mp("rosso0"), mp("rosso1")},
			"red.Nonymous.I":   PropertySlice{mp(0), mp(0)},
			"red.Other":        mp(""),
			"green.S":          mp("vert"),
			"green.I":          mp(0),
			"green.Nonymous.S": PropertySlice{mp("verde0"), mp("verde1"), mp("verde2")},
			"green.Nonymous.I": PropertySlice{mp(0), mp(0), mp(0)},
			"green.Other":      mp(""),
			"Blue.S":           mp("bleu"),
			"Blue.I":           mp(0),
			"Blue.Nonymous.S":  PropertySlice{mp("blu0"), mp("blu1"), mp("blu2"), mp("blu3")},
			"Blue.Nonymous.I":  PropertySlice{mp(0), mp(0), mp(0), mp(0)},
			"Blue.Other":       mp(""),
		},
	},
	{
		desc: "save props load structs with ragged fields",
		src: PropertyMap{
			"red.S":            mp("rot"),
			"green.Nonymous.I": PropertySlice{mp(10), mp(11), mp(12), mp(13)},
			"Blue.Nonymous.S":  PropertySlice{mp("blau0"), mp("blau1"), mp("blau2")},
			"Blue.Nonymous.I":  PropertySlice{mp(20), mp(21)},
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
		desc: "deeper slices",
		src: &DeeperSlice{
			A: []DeeperA{
				{
					B: DeeperB{"v1"},
				},
				{
					B: DeeperB{"v2"},
				},
			},
		},
		want: &DeeperSlice{
			A: []DeeperA{
				{
					B: DeeperB{"v1"},
				},
				{
					B: DeeperB{"v2"},
				},
			},
		},
	},
	{
		desc: "lsp indexed",
		src: &LSPOuter{
			One: LSPInner{
				Kind:        "Test",
				ID:          "test",
				Ints:        []int64{1, 2},
				IntsNoIndex: []int64{3, 4},
				Deeper1: LSPDeeper{
					ID:    123,
					Field: 5,
				},
			},
		},
		want: PropertyMap{
			"One": mp(PropertyMap{
				"$kind":       mpNI("Test"),
				"$id":         mpNI("test"),
				"Ints":        PropertySlice{mp(1), mp(2)},
				"IntsNoIndex": PropertySlice{mpNI(3), mpNI(4)},
				"Bool":        mp(false),
				"Blob":        mp([]byte(nil)),
				"String":      mp(""),
				"Double":      mp(0.0),
				"Deeper1": mp(PropertyMap{
					"$kind": mpNI("LSPDeeper"),
					"$id":   mpNI(123),
					"Field": mp(5),
				}),
			}),
		},
	},
	{
		desc: "lsp unindexed",
		src: &LSPOuterUnindexed{
			One: LSPInner{
				Kind:        "Test",
				ID:          "test",
				Ints:        []int64{1, 2},
				IntsNoIndex: []int64{3, 4},
				Deeper1: LSPDeeper{
					ID:    123,
					Field: 5,
				},
			},
		},
		want: PropertyMap{
			"One": mpNI(PropertyMap{
				"$kind":       mpNI("Test"),
				"$id":         mpNI("test"),
				"Ints":        PropertySlice{mpNI(1), mpNI(2)},
				"IntsNoIndex": PropertySlice{mpNI(3), mpNI(4)},
				"Bool":        mpNI(false),
				"Blob":        mpNI([]byte(nil)),
				"String":      mpNI(""),
				"Double":      mpNI(0.0),
				"Deeper1": mpNI(PropertyMap{
					"$kind": mpNI("LSPDeeper"),
					"$id":   mpNI(123),
					"Field": mpNI(5),
				}),
			}),
		},
	},
	{
		desc: "lsp round trip",
		src: &LSPOuter{
			One: LSPInner{
				Kind: "Test",
				ID:   "test",
				Ints: []int64{1, 2, 3},
			},
			Slice: []LSPInner{
				{
					Ints:    []int64{4, 5},
					Deeper1: LSPDeeper{ID: 1},
					Deeper2: []LSPDeeper{{ID: 2}, {ID: 3}},
				},
				{
					Ints:    []int64{6},
					Deeper2: []LSPDeeper{{ID: 4}},
				},
				{},
			},
		},
		want: &LSPOuter{
			One: LSPInner{
				Kind: "Test",
				ID:   "test",
				Ints: []int64{1, 2, 3},
			},
			Slice: []LSPInner{
				{
					Ints:    []int64{4, 5},
					Deeper1: LSPDeeper{ID: 1},
					Deeper2: []LSPDeeper{{ID: 2}, {ID: 3}},
				},
				{
					Ints:    []int64{6},
					Deeper2: []LSPDeeper{{ID: 4}},
				},
				{},
			},
		},
	},
	{
		desc: "lsp wrong type",
		src: PropertyMap{
			"One": mpNI(123),
		},
		want:    &LSPOuter{},
		loadErr: "expecting a property map or null",
	},
	{
		desc: "lsp null",
		src: PropertyMap{
			"One": mpNI(nil),
		},
		want: &LSPOuter{},
	},
	{
		desc: "legacy LSP",
		src: PropertyMap{
			"Slice": PropertySlice{
				mp(deflate(legacyLSP(
					legacyProp("Ints", int64(1)),
					legacyProp("Ints", int64(2)),
					legacyProp("Bool", true),
					legacyProp("Blob", []byte("hi")),
					legacyProp("String", "str"),
					legacyProp("Double", float64(12.34)),
					legacyProp("Deeper2", legacyLSP(
						legacyProp("Field", int64(111)),
					)),
					legacyProp("Deeper2", deflate(legacyLSP(
						legacyProp("Field", int64(222)),
					))),
				))),
			},
		},
		want: &LSPOuter{
			Slice: []LSPInner{
				{
					Ints:   []int64{1, 2},
					Bool:   true,
					Blob:   []byte("hi"),
					String: "str",
					Double: 12.34,
					Deeper2: []LSPDeeper{
						{Field: 111},
						{Field: 222},
					},
				},
			},
		},
	},
	{
		desc: "storing unset optionals",
		src:  &Optionals{},
		want: PropertyMap{},
	},
	{
		desc: "storing set optionals",
		src: &Optionals{
			Int:       NewIndexedOptional(int64(123)),
			IntAlias:  NewIndexedOptional(IntAlias(456)),
			Unindexed: NewUnindexedOptional(int64(789)),
			String:    NewIndexedOptional("hi"),
			Bytes:     NewIndexedOptional([]byte("bla")),
			Time:      NewIndexedOptional(time.Time{}),
			Geo:       NewIndexedOptional(testGeoPt0),
			Key:       NewIndexedOptional(testKey0),
			Slice: []Optional[int64, Indexed]{
				NewIndexedOptional(int64(666)),
				NewIndexedOptional(int64(777)),
			},
		},
		want: PropertyMap{
			"Int":       mp(int64(123)),
			"IntAlias":  mp(int64(456)),
			"Unindexed": mpNI(int64(789)),
			"String":    mp("hi"),
			"Bytes":     mp([]byte("bla")),
			"Time":      mp(time.Time{}),
			"Geo":       mp(testGeoPt0),
			"Key":       mp(testKey0),
			"Slice":     PropertySlice{mp(int64(666)), mp(int64(777))},
		},
	},
	{
		desc: "loading optionals",
		src: PropertyMap{
			"Int":       mp(int64(123)),
			"IntAlias":  mp(int64(456)),
			"Unindexed": mpNI(int64(789)),
			"String":    mp("hi"),
			"Bytes":     mp([]byte("bla")),
			"Time":      mp(time.Time{}),
			"Geo":       mp(testGeoPt0),
			"Key":       mp(testKey0),
			"Slice":     PropertySlice{mp(int64(666)), mp(int64(777))},
		},
		want: &Optionals{
			Int:       NewIndexedOptional(int64(123)),
			IntAlias:  NewIndexedOptional(IntAlias(456)),
			Unindexed: NewUnindexedOptional(int64(789)),
			String:    NewIndexedOptional("hi"),
			Bytes:     NewIndexedOptional([]byte("bla")),
			Time:      NewIndexedOptional(time.Time{}),
			Geo:       NewIndexedOptional(testGeoPt0),
			Key:       NewIndexedOptional(testKey0),
			Slice: []Optional[int64, Indexed]{
				NewIndexedOptional(int64(666)),
				NewIndexedOptional(int64(777)),
			},
		},
	},
	{
		desc: "loading null into optional",
		src: PropertyMap{
			"Int":   mp(nil),
			"Slice": PropertySlice{mp(int64(666)), mp(nil), mp(int64(777))},
		},
		want: &Optionals{
			Slice: []Optional[int64, Indexed]{
				NewIndexedOptional(int64(666)),
				{}, // unset nil
				NewIndexedOptional(int64(777)),
			},
		},
	},
	{
		desc: "storing unset nullables",
		src:  &Nullables{},
		want: PropertyMap{
			"Indexed":   mp(nil),
			"Unindexed": mpNI(nil),
		},
	},
	{
		desc: "load into unexported fields",
		src: PropertyMap{
			"Private": mp("s"),
		},
		want:    &Unexported{private: "str"},
		loadErr: "no such struct field",
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
		want: PropertyMap{
			"B.Y": mp(""),
			"A.X": mpNI(""),
			"A.Y": mpNI(""),
			"B.X": mpNI(""),
		},
	},
	{
		desc: "embedded struct with name override",
		src: &struct {
			Inner1 `gae:"foo"`
		}{},
		want: PropertyMap{
			"foo.W": mp(0),
			"foo.X": mp(""),
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
		want: PropertyMap{
			"J": mp(2),
		},
	},
	{
		desc: "json.RawMessage",
		src: &struct {
			J json.RawMessage
		}{
			J: json.RawMessage("rawr"),
		},
		want: PropertyMap{
			"J": mp([]byte("rawr")),
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
	{
		desc:     "protos encoding: NULL property deserializes into nil",
		src:      PropertyMap{"Msg": mpNI(nil)},
		loadInto: &WithProtoDefault{Msg: &testprotos.Msg{}},
		want:     &WithProtoDefault{},
	},
	{
		desc: "protos encoding: default is nocompression until 16KiB",
		src:  &WithProtoDefault{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
		want: PropertyMap{
			"Msg": mpNI([]byte{
				protoBinOptNoCompress, // no compression varint
				// data
				10, 20, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97,
			}),
		},
	},
	{
		desc: "protos encoding: default is nocompression until 16KiB: save/load",
		src: &WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		want: &WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
	},
	{
		desc: "protos encoding: default is compression after 16KiB",
		src:  &WithProtoDefault{Msg: &testprotos.Msg{Str: strings.Repeat("a", 16*1024)}},
		want: PropertyMap{
			"Msg": mpNI([]byte{
				protoBinOptZSTD,
				// compressed data
				40, 181, 47, 253, 100, 4, 63, 109, 0, 0, 40, 10, 128, 128, 1, 97, 1, 84, 5, 2, 49, 252, 159, 133, 80, 47, 43,
			}),
		},
	},
	{
		desc: "protos encoding: default is compression after 16KiB: save/load",
		src:  &WithProtoDefault{Msg: &testprotos.Msg{Str: strings.Repeat("a", 16*1024)}},
		want: &WithProtoDefault{Msg: &testprotos.Msg{Str: strings.Repeat("a", 16*1024)}},
	},
	{
		desc: "protos encoding: compression",
		src:  &WithProtoCompress{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
		want: PropertyMap{
			"Msg": mpNI([]byte{
				protoBinOptZSTD,
				// compressed data
				40, 181, 47, 253, 4, 0, 85, 0, 0, 24, 10, 20, 97, 1, 84, 3, 2, 16, 4, 246, 66, 60, 172,
			}),
		},
	},
	{
		desc: "protos encoding: compression: save/load",
		src:  &WithProtoCompress{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
		want: &WithProtoCompress{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
	},
	{
		desc: "protos encoding: compression: save/load of empty proto",
		src:  &WithProtoCompress{Msg: &testprotos.Msg{}},
		want: &WithProtoCompress{Msg: &testprotos.Msg{}},
	},
	{
		desc: "protos encoding: interop save/load zstd -> plain ",
		src: &WithProtoCompress{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		want: &WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
	},
	{
		desc: "protos encoding: interop save/load uncompress -> ztd",
		src: &WithProtoCompress{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		want: &WithProtoCompress{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
	},
	{
		desc: "protos encoding: legacy",
		src:  &WithProtoLegacy{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
		want: PropertyMap{
			"Msg": mpNI([]byte{
				// just data
				10, 20, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97, 97,
			}),
		},
	},
	{
		desc: "protos encoding: legacy: save/load",
		src:  &WithProtoLegacy{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
		want: &WithProtoLegacy{Msg: &testprotos.Msg{Str: strings.Repeat("a", 20)}},
	},
	{
		desc: "protos encoding: no interop: read as legacy",
		src: &WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		want: &WithProtoLegacy{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		loadErr: `gae: cannot load field "Msg" into a "datastore.WithProtoLegacy`,
	},
	{
		desc: "protos encoding: no interop: read as modern",
		src: &WithProtoLegacy{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		want: &WithProtoCompress{
			Msg: &testprotos.Msg{Str: "abcdef", Dur: &durationpb.Duration{Seconds: 251, Nanos: 252}},
		},
		loadErr: `gae: cannot load field "Msg" into a "datastore.WithProtoCompress`,
	},
	{
		desc: "protos encoding: - nil is omitted from PropertyMap",
		src:  &WithProtoDefault{Msg: nil},
		want: PropertyMap{},
	},
	{
		desc: "protos encoding: - nil is omitted from PropertyMap: save/load",
		src:  &WithProtoDefault{Msg: nil},
		want: &WithProtoDefault{Msg: nil},
	},
	{
		desc: "protos encoding: - empty msg is kept in PropertyMap",
		src:  &WithProtoDefault{Msg: &testprotos.Msg{}},
		want: PropertyMap{
			"Msg": mpNI([]byte{protoBinOptNoCompress}), // just prefix.
		},
	},
	{
		desc: "protos encoding: - empty msg is kept in PropertyMap: save/load",
		src:  &WithProtoDefault{Msg: &testprotos.Msg{}},
		want: &WithProtoDefault{Msg: &testprotos.Msg{}},
	},
	{
		desc: "protos encoding: - (omit) option is respected",
		src:  &WithProtoOmitted{Omitted: &testprotos.Msg{Str: "omit"}},
		want: &WithProtoOmitted{Omitted: nil},
	},
	{
		desc: "protos are always no index",
		src: &struct {
			Redundant *testprotos.Msg `gae:",noindex"`
		}{},
		plsErr: `noindex option is redundant and not allowed with proto.Message "Redundant"`,
	},
	{
		desc: "protos with unexpected option",
		src: &struct {
			Wrong *testprotos.Msg `gae:",wrong"`
		}{},
		plsErr: `unsupported option "wrong" for proto.Message "Wrong"`,
	},
	{
		desc:   "protos: slice not supported, use repeated instead",
		src:    &struct{ Slice []*testprotos.Msg }{Slice: []*testprotos.Msg{{Str: "skipped"}}},
		plsErr: `repeated protobuf fields like "Slice" are not supported`,
	},
	{
		desc: "protos: embed struct with protos",
		src: &WithProtoIndirect{Inner: WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abc"},
		}},
		want: PropertyMap{
			"Inner.Msg": mpNI([]byte{protoBinOptNoCompress, 10, 3, 97, 98, 99}),
		},
	},
	{
		desc: "protos: embed struct with protos: save/load",
		src: &WithProtoIndirect{Inner: WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abc"},
		}},
		want: &WithProtoIndirect{Inner: WithProtoDefault{
			Msg: &testprotos.Msg{Str: "abc"},
		}},
	},
	{
		desc: "protos: embed slice of struct with protos",
		src: &WithProtoIndirectSlice{Inner: []WithProtoDefault{
			{Msg: &testprotos.Msg{Str: "abc"}},
			{Msg: &testprotos.Msg{Str: "xyz"}},
		}},
		want: PropertyMap{
			"Inner.Msg": PropertySlice{
				mpNI([]byte{protoBinOptNoCompress, 10, 3, 97, 98, 99}),
				mpNI([]byte{protoBinOptNoCompress, 10, 3, 120, 121, 122}),
			},
		},
	},
	{
		desc: "protos: embed slice of struct with protos: save/load",
		src: &WithProtoIndirectSlice{Inner: []WithProtoDefault{
			{Msg: &testprotos.Msg{Str: "abc"}},
			{Msg: &testprotos.Msg{Str: "xyz"}},
		}},
		want: &WithProtoIndirectSlice{Inner: []WithProtoDefault{
			{Msg: &testprotos.Msg{Str: "abc"}},
			{Msg: &testprotos.Msg{Str: "xyz"}},
		}},
	},
	{
		desc:   "protos: embedding anonymous proto is not allowed",
		src:    &WithAnonymousProto{&testprotos.Msg{Str: "xyz"}},
		plsErr: `non-struct embedded field: "Msg"`,
	},
	{
		desc: "resets structs before loading",
		src: &N0{
			X0: X0{S: "one", I: 2},
		},
		loadInto: &N0{
			X0:       X0{S: "one", I: 2, i: 3},
			Nonymous: X0{S: "four", I: 5, i: 6}, // must be reset
			Ignore:   "ignore",
			Other:    "other", // must be reset
		},
		want: &N0{
			X0:       X0{S: "one", I: 2, i: 3},
			Nonymous: X0{i: 6},
			Ignore:   "ignore",
		},
	},
	{
		desc: "structs with slices of structs are reset on load",
		src: &N1{
			X0: X0{S: "rouge"},
			Nonymous: []X0{
				{S: "first"},
				{S: "second"},
			},
		},
		loadInto: &N1{
			X0: X0{S: "bleu"},
			Nonymous: []X0{
				{S: "garbage"},
			},
		},
		want: &N1{ // exact same as `src`
			X0: X0{S: "rouge"},
			Nonymous: []X0{
				{S: "first"},
				{S: "second"},
			},
		},
	},
	{
		desc:     "structs with slices of base type are reset on load",
		src:      &B1{B: []int8{1, 2}},
		loadInto: &B1{B: []int8{4}},
		want:     &B1{B: []int8{1, 2}},
	},
	{
		desc: "structs are reset excluding $id and $parent",
		src: &StructForReset{
			ID:     "saved ID",
			Parent: testKey1a,
		},
		loadInto: &StructForReset{
			// These fields are reset.
			Field:  "pre-field",
			Struct: SubStructForReset{SubSlice: []string{"to", "be", "reset"}},
			Slice:  []string{"pre", "slice"},
			Extra:  PropertyMap{"should": mpNI("be reset")},
			Opt:    NewIndexedOptional(int64(123)),
			// $id and $parent preserved as is.
			ID:     "not reset ID",
			Parent: testKey2a,
		},
		want: &StructForReset{
			ID:     "not reset ID",
			Parent: testKey2a,
		},
	},
	{
		desc: "structs are reset, recursively, but excluding private and non-GAE exported fields",
		src: &StructForReset{
			ID:     "ID",
			Parent: testKey1a,
			Slice:  []string{"slice", "of", "strings"},
			Extra:  PropertyMap{"saved": mpNI("value")},
		},
		loadInto: &StructForReset{
			ID:     "ID",
			Parent: testKey1a,
			Field:  "pre-field",
			Struct: SubStructForReset{
				subPrivate: 42,
				SubSlice:   []string{"to", "be", "reset"},
			},
			Slice: []string{"pre", "slice"},
			Extra: PropertyMap{"should": mpNI("be reset")},

			Skip:          "must be preserved",
			private:       42,
			privateSlice:  []int{42},
			privateStruct: SubStructForReset{SubSlice: []string{"preserved", "private in parent"}, subPrivate: 42},
		},
		want: &StructForReset{
			// The only two fields which are read from `src` is Slice & Extra.
			Slice: []string{"slice", "of", "strings"},
			// Note that `extra` PropertyMap is generalized to field -> slice.
			Extra: PropertyMap{"saved": PropertySlice{mpNI("value")}},

			// These were reset in `loadInto`, except private field inside `Struct`.
			Field:  "",
			Struct: SubStructForReset{subPrivate: 42},

			// The rest is the same as in `loadInto`.
			ID:            "ID",
			Parent:        testKey1a,
			Skip:          "must be preserved",
			private:       42,
			privateSlice:  []int{42},
			privateStruct: SubStructForReset{SubSlice: []string{"preserved", "private in parent"}, subPrivate: 42},
		},
	},
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	getPLSErr := func(obj any) (pls PropertyLoadSaver, err error) {
		defer func() {
			if v := recover(); v != nil {
				err = v.(error)
			}
		}()
		pls = GetPLS(obj)
		return
	}

	ftt.Run("Test round-trip", t, func(t *ftt.Test) {
		for _, tc := range testCases {
			t.Run(tc.desc, func(t *ftt.Test) {
				pls, ok := tc.src.(PropertyLoadSaver)
				if !ok {
					var err error
					pls, err = getPLSErr(tc.src)
					if tc.plsErr != "" {
						assert.Loosely(t, err, should.ErrLike(tc.plsErr))
						return
					}
					assert.Loosely(t, err, should.BeNil)
				}
				assert.Loosely(t, pls, should.NotBeNil)

				savedProps, err := pls.Save(false)
				if tc.saveErr != "" {
					assert.Loosely(t, err, should.ErrLike(tc.saveErr))
					return
				}
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, savedProps, should.NotBeNil)

				var got any
				if _, ok := tc.want.(PropertyMap); ok {
					pls = PropertyMap{}
					got = pls
				} else {
					if tc.loadInto != nil {
						got = tc.loadInto
					} else {
						got = reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
					}
					if pls, ok = got.(PropertyLoadSaver); !ok {
						var err error
						pls, err = getPLSErr(got)
						if tc.plsLoadErr != "" {
							assert.Loosely(t, err, should.ErrLike(tc.plsLoadErr))
							return
						}
						assert.Loosely(t, err, should.BeNil)
					}
				}

				assert.Loosely(t, pls, should.NotBeNil)

				err = pls.Load(savedProps)
				if tc.loadErr != "" {
					assert.Loosely(t, err, should.ErrLike(tc.loadErr))
					return
				}
				// Second load should not change anything.
				assert.Loosely(t, pls.Load(savedProps), should.BeNil)
				if tc.want == nil {
					return
				}

				if gotT, ok := got.(*T); ok {
					// Round tripping a time.Time can result in a different time.Location: Local instead of UTC.
					// We therefore test equality explicitly, instead of relying on reflect.DeepEqual.
					assert.Loosely(t, gotT.T.Equal(tc.want.(*T).T), should.BeTrue)
					return
				}
				if _, ok := tc.want.(protoEmbedder); ok {
					assert.Loosely(t, got, should.Resemble(tc.want))
					return
				}
				assert.Loosely(t, got, should.Resemble(tc.want))
			})
		}
	})
}

func TestMeta(t *testing.T) {
	t.Parallel()

	ftt.Run("Test meta fields", t, func(t *ftt.Test) {
		t.Run("Can retrieve from struct", func(t *ftt.Test) {
			o := &N0{ID: 100}
			mgs := getMGS(o)
			val, ok := mgs.GetMeta("id")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Equal(100))

			val, ok = mgs.GetMeta("kind")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Equal("whatnow"))

			assert.Loosely(t, GetMetaDefault(mgs, "kind", "zappo"), should.Equal("whatnow"))
			assert.Loosely(t, GetMetaDefault(mgs, "id", "stringID"), should.Equal("stringID"))
			assert.Loosely(t, GetMetaDefault(mgs, "id", 6), should.Equal(100))
		})

		t.Run("Getting something not there is an error", func(t *ftt.Test) {
			o := &N0{ID: 100}
			mgs := getMGS(o)
			_, ok := mgs.GetMeta("wat")
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Default works for missing fields", func(t *ftt.Test) {
			o := &N0{ID: 100}
			mgs := getMGS(o)
			assert.Loosely(t, GetMetaDefault(mgs, "whozit", 10), should.Equal(10))
		})

		t.Run("getting mgs for bad struct is an error", func(t *ftt.Test) {
			assert.Loosely(t, func() { getMGS(&Recursive{}) }, should.PanicLike(
				`field "R" is recursively defined`))
		})

		t.Run("can assign values to exported meta fields", func(t *ftt.Test) {
			o := &N0{ID: 100}
			mgs := getMGS(o)
			assert.Loosely(t, mgs.SetMeta("id", int64(200)), should.BeTrue)
			assert.Loosely(t, o.ID, should.Equal(200))
		})

		t.Run("assigning to unsassiagnable fields returns !ok", func(t *ftt.Test) {
			o := &N0{ID: 100}
			mgs := getMGS(o)
			assert.Loosely(t, mgs.SetMeta("kind", "hi"), should.BeFalse)
			assert.Loosely(t, mgs.SetMeta("noob", "hi"), should.BeFalse)
		})

		t.Run("getting unexported meta", func(t *ftt.Test) {
			o := &N0{ID: 100, _kind: "Override"}
			mgs := getMGS(o)
			val, ok := mgs.GetMeta("kind")
			assert.Loosely(t, ok, should.BeTrue)
			// TODO(vadimsh): This is potentially a bug.
			assert.Loosely(t, val, should.Match("whatnow"))
		})

		t.Run("protos get/set", func(t *ftt.Test) {
			p := &WithProtoMeta{}
			assert.Loosely(t, getMGS(p).SetMeta("what", []byte{
				protoBinOptZSTD,
				40, 181, 47, 253, 4, 0, 49, 0, 0, 10, 4, 97, 97, 97, 97, 115, 247, 182, 146}),
				should.BeTrue)
			assert.Loosely(t, p.Msg.GetStr(), should.Equal("aaaa"))
			val, ok := getMGS(p).GetMeta("what")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Resemble([]byte{protoBinOptNoCompress, 10, 4, 97, 97, 97, 97}))
		})

		t.Run("unsigned int meta fields work", func(t *ftt.Test) {
			o := &N3{}
			mgs := getMGS(o)
			v, ok := mgs.GetMeta("id")
			assert.Loosely(t, v, should.Equal(int64(200)))
			assert.Loosely(t, ok, should.BeTrue)

			assert.Loosely(t, mgs.SetMeta("id", 20), should.BeTrue)
			assert.Loosely(t, o.ID, should.Equal(20))

			assert.Loosely(t, mgs.SetMeta("id", math.MaxInt64), should.BeFalse)
			assert.Loosely(t, o.ID, should.Equal(20))

			assert.Loosely(t, mgs.SetMeta("id", math.MaxUint32), should.BeTrue)
			assert.Loosely(t, o.ID, should.Equal(math.MaxUint32))
		})
	})

	ftt.Run("StructPLS Miscellaneous", t, func(t *ftt.Test) {
		t.Run("a simple struct has a default $kind", func(t *ftt.Test) {
			assert.Loosely(t, GetPLS(&Simple{}).GetAllMeta(), should.Resemble(PropertyMap{
				"$kind": mpNI("Simple"),
			}))
		})

		t.Run("multiple overlapping fields is an error", func(t *ftt.Test) {
			o := &BadMeta{}
			assert.Loosely(t, func() { GetPLS(o) }, should.PanicLike("multiple times"))
		})

		t.Run("empty property names are invalid", func(t *ftt.Test) {
			assert.Loosely(t, validPropertyName(""), should.BeFalse)
		})

		t.Run("attempting to get a PLS for a non *struct is an error", func(t *ftt.Test) {
			s := []string{}
			assert.Loosely(t, func() { GetPLS(&s) }, should.PanicLike(
				"cannot GetPLS(*[]string): not a pointer-to-struct"))
		})

		t.Run("attempting to get a PLS for a nil pointer-to-struct is an error", func(t *ftt.Test) {
			var s *Simple
			assert.Loosely(t, func() { GetPLS(s) }, should.PanicLike(
				"cannot GetPLS(*datastore.Simple): pointer is nil"))
		})

		t.Run("convertible meta default types", func(t *ftt.Test) {
			type OKDefaults struct {
				When   string `gae:"$when,tomorrow"`
				Amount int64  `gae:"$amt,100"`
				DoIt   Toggle `gae:"$doit,on"`
			}
			okd := &OKDefaults{}
			mgs := getMGS(okd)

			v, ok := mgs.GetMeta("when")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal("tomorrow"))

			v, ok = mgs.GetMeta("amt")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal(int64(100)))

			assert.Loosely(t, okd.DoIt, should.Equal(Auto))
			v, ok = mgs.GetMeta("doit")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.BeTrue)

			assert.Loosely(t, mgs.SetMeta("doit", false), should.BeTrue)
			v, ok = mgs.GetMeta("doit")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.BeFalse)
			assert.Loosely(t, okd.DoIt, should.Equal(Off))

			assert.Loosely(t, mgs.SetMeta("doit", true), should.BeTrue)
			v, ok = mgs.GetMeta("doit")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.BeTrue)
			assert.Loosely(t, okd.DoIt, should.Equal(On))

			t.Run("Toggle fields REQUIRE a default", func(t *ftt.Test) {
				type BadToggle struct {
					Bad Toggle `gae:"$wut"`
				}
				assert.Loosely(t, func() { GetPLS(&BadToggle{}) }, should.PanicLike("bad/missing default"))
			})
		})

		t.Run("meta fields can be saved", func(t *ftt.Test) {
			type OKDefaults struct {
				When   string `gae:"$when,tomorrow"`
				Amount int64  `gae:"$amt,100"`
			}
			pls := GetPLS(&OKDefaults{})
			pm, err := pls.Save(true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pm, should.Resemble(PropertyMap{
				"$when": mpNI("tomorrow"),
				"$amt":  mpNI(100),
				"$kind": mpNI("OKDefaults"),
			}))

			v, ok := pm.GetMeta("when")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal("tomorrow"))

			v, ok = pm.GetMeta("amt")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal(int64(100)))
		})

		t.Run("default are optional", func(t *ftt.Test) {
			type OverrideDefault struct {
				Val int64 `gae:"$val"`
			}
			o := &OverrideDefault{}
			mgs := getMGS(o)

			v, ok := mgs.GetMeta("val")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal(int64(0)))
		})

		t.Run("overridable defaults", func(t *ftt.Test) {
			type OverrideDefault struct {
				Val int64 `gae:"$val,100"`
			}
			o := &OverrideDefault{}
			mgs := getMGS(o)

			v, ok := mgs.GetMeta("val")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal(int64(100)))

			o.Val = 10
			v, ok = mgs.GetMeta("val")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal(int64(10)))
		})

		t.Run("underflow", func(t *ftt.Test) {
			type UnderflowMeta struct {
				ID int16 `gae:"$id"`
			}
			um := &UnderflowMeta{}
			mgs := getMGS(um)
			assert.Loosely(t, mgs.SetMeta("id", -20), should.BeTrue)
			assert.Loosely(t, mgs.SetMeta("id", math.MinInt64), should.BeFalse)
		})

		t.Run("negative default", func(t *ftt.Test) {
			type UnderflowMeta struct {
				ID int16 `gae:"$id,-30"`
			}
			um := &UnderflowMeta{}
			mgs := getMGS(um)
			val, ok := mgs.GetMeta("id")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Equal(-30))
		})

		t.Run("Derived metadata fields", func(t *ftt.Test) {
			type DerivedString string
			type DerivedInt int16
			type DerivedStruct struct {
				ID  DerivedString `gae:"$id"`
				Foo DerivedInt    `gae:"$foo"`
			}
			o := &DerivedStruct{"hello", 10}
			mgs := getMGS(o)
			v, ok := mgs.GetMeta("id")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal("hello"))

			v, ok = mgs.GetMeta("foo")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal(int64(10)))

			assert.Loosely(t, mgs.SetMeta("id", "nerds"), should.BeTrue)
			assert.Loosely(t, mgs.SetMeta("foo", 20), should.BeTrue)
			assert.Loosely(t, o.ID, should.Equal(DerivedString("nerds")))
			assert.Loosely(t, o.Foo, should.Equal(DerivedInt(20)))
		})

		t.Run("Bad default meta type", func(t *ftt.Test) {
			type BadDefault struct {
				Val time.Time `gae:"$meta,tomorrow"`
			}
			assert.Loosely(t, func() { GetPLS(&BadDefault{}) }, should.PanicLike("bad type"))
		})

		t.Run("MetaGetterSetter implementation (IDParser)", func(t *ftt.Test) {
			idp := &IDParser{parent: "moo", id: 100}
			mgs := getMGS(idp)
			assert.Loosely(t, GetMetaDefault(mgs, "id", ""), should.Equal("moo|100"))

			assert.Loosely(t, GetMetaDefault(mgs, "kind", ""), should.Equal("CoolKind"))

			assert.Loosely(t, mgs.SetMeta("kind", "Something"), should.BeFalse)
			assert.Loosely(t, mgs.SetMeta("id", "happy|27"), should.BeTrue)

			assert.Loosely(t, idp.parent, should.Equal("happy"))
			assert.Loosely(t, idp.id, should.Equal(27))

			assert.Loosely(t, mgs.GetAllMeta(), should.Resemble(PropertyMap{
				"$id":   mpNI("happy|27"),
				"$kind": mpNI("CoolKind"),
			}))
		})

		t.Run("MetaGetterSetter implementation (KindOverride)", func(t *ftt.Test) {
			ko := &KindOverride{ID: 20}
			mgs := getMGS(ko)
			assert.Loosely(t, GetMetaDefault(mgs, "kind", ""), should.Equal("KindOverride"))

			ko.customKind = "something"
			assert.Loosely(t, GetMetaDefault(mgs, "kind", ""), should.Equal("something"))

			assert.Loosely(t, mgs.SetMeta("kind", "Nerp"), should.BeTrue)
			assert.Loosely(t, ko.customKind, should.Equal("Nerp"))

			assert.Loosely(t, mgs.SetMeta("kind", "KindOverride"), should.BeTrue)
			assert.Loosely(t, ko.customKind, should.BeEmpty)

			assert.Loosely(t, mgs.GetAllMeta(), should.Resemble(PropertyMap{
				"$id":   mpNI(20),
				"$kind": mpNI("KindOverride"),
			}))
			ko.customKind = "wut"
			assert.Loosely(t, mgs.GetAllMeta(), should.Resemble(PropertyMap{
				"$id":   mpNI(20),
				"$kind": mpNI("wut"),
			}))

			props, err := GetPLS(ko).Save(true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, props, should.Resemble(PropertyMap{
				"$id":   mpNI(20),
				"$kind": mpNI("wut"),
			}))
		})

		t.Run("Embeddable Metadata structs", func(t *ftt.Test) {
			ide := &IDEmbedder{EmbeddedID{"hello", 10}}
			pls := GetPLS(ide)
			val, ok := pls.GetMeta("id")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, val, should.Equal("hello|10"))

			assert.Loosely(t, pls.SetMeta("id", "sup|1337"), should.BeTrue)
			assert.Loosely(t, ide.EmbeddedID, should.Resemble(EmbeddedID{"sup", 1337}))

			assert.Loosely(t, pls.GetAllMeta(), should.Resemble(PropertyMap{
				"$id":   mpNI("sup|1337"),
				"$kind": mpNI("IDEmbedder"),
			}))
		})
	})
}
