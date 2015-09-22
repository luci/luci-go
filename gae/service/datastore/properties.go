// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/luci/gae/service/blobstore"
)

var (
	minTime = time.Unix(int64(math.MinInt64)/1e6, (int64(math.MinInt64)%1e6)*1e3)
	maxTime = time.Unix(int64(math.MaxInt64)/1e6, (int64(math.MaxInt64)%1e6)*1e3)

	utcTestTime = time.Unix(0, 0)
)

// IndexSetting indicates whether or not a Property should be indexed by the
// datastore.
type IndexSetting bool

// ShouldIndex is the default, which is why it must assume the zero value,
// even though it's werid :(.
const (
	ShouldIndex IndexSetting = false
	NoIndex     IndexSetting = true
)

func (i IndexSetting) String() string {
	if i {
		return "NoIndex"
	}
	return "ShouldIndex"
}

// PropertyConverter may be implemented by the pointer-to a struct field which
// is serialized by the struct PropertyLoadSaver from GetPLS. Its ToProperty
// will be called on save, and it's FromProperty will be called on load (from
// datastore). The method may do arbitrary computation, and if it encounters an
// error, may return it.  This error will be a fatal error (as defined by
// PropertyLoadSaver) for the struct conversion.
//
// Example:
//   type Complex complex
//   func (c *Complex) ToProperty() (ret Property, err error) {
//     // something like:
//     err = ret.SetValue(fmt.Sprint(*c), true)
//     return
//   }
//   func (c *Complex) FromProperty(p Property) (err error) {
//     ... load *c from p ...
//   }
//
//   type MyStruct struct {
//     Complexity []Complex // acts like []complex, but can be serialized to DS
//   }
type PropertyConverter interface {
	// TODO(riannucci): Allow a convertable to return multiple values.  This is
	// eminently doable (as long as the single-slice restriction is kept).  It
	// could also cut down on the amount of reflection necessary when resolving
	// a path in a struct (in the struct loading routine in helper).

	ToProperty() (Property, error)
	FromProperty(Property) error
}

// PropertyType is a single-byte representation of the type of data contained
// in a Property. The specific values of this type information are chosen so
// that the types sort according to the order of types as sorted by the
// datastore.
//
// Note that indexes may only contain values of the following types:
//   PTNull
//   PTInt
//   PTBool
//   PTFloat
//   PTString
//   PTGeoPoint
//   PTKey
//
// The biggest impact of this is that if you do a Projection query, you'll only
// get back Properties with the above types (e.g. if you store a PTTime value,
// then Project on it, you'll get back a PTInt value). For convenience, Property
// has a Project(PropertyType) method which will side-cast to your intended
// type. If you project into a structure with the high-level Interface
// implementation, or use StructPLS, this conversion will be done for you
// automatically, using the type of the destination field to cast.
type PropertyType byte

//go:generate stringer -type=PropertyType

// These constants are in the order described by
//   https://cloud.google.com/appengine/docs/go/datastore/entities#Go_Value_type_ordering
// with a slight divergence for the Int/Time split.
//
// NOTE: this enum can only occupy 7 bits, because we use the high bit to encode
// indexed/non-indexed, and we additionally require that all valid values and
// all INVERTED valid values must never equal 0xFF or 0x00. The reason for this
// constraint is that we must always be able to create a byte that sorts before
// and after it.
//
// See "./serialize".WriteProperty and "impl/memory".increment for more info.
const (
	// PTNull represents the 'nil' value. This is only directly visible when
	// reading/writing a PropertyMap. If a PTNull value is loaded into a struct
	// field, the field will be initialized with its zero value. If a struct with
	// a zero value is saved from a struct, it will still retain the field's type,
	// not the 'nil' type. This is in contrast to other GAE languages such as
	// python where 'None' is a distinct value than the 'zero' value (e.g. a
	// StringProperty can have the value "" OR None).
	//
	// PTNull is a Projection-query type
	PTNull PropertyType = iota

	// PTInt is always an int64.
	//
	// This is a Projection-query type, and may be projected to PTTime.
	PTInt
	PTTime

	// PTBool represents true or false
	//
	// This is a Projection-query type.
	PTBool

	// PTBytes represents []byte
	PTBytes

	// PTString is used to represent all strings (text).
	//
	// PTString is a Projection-query type and may be projected to PTBytes or
	// PTBlobKey.
	PTString

	// PTFloat is always a float64.
	//
	// This is a Projection-query type.
	PTFloat

	// PTGeoPoint is a Projection-query type.
	PTGeoPoint

	// PTKey represents a *Key object.
	//
	// PTKey is a Projection-query type.
	PTKey

	// PTBlobKey represents a blobstore.Key
	PTBlobKey

	// PTUnknown is a placeholder value which should never show up in reality.
	//
	// NOTE: THIS MUST BE LAST VALUE FOR THE init() ASSERTION BELOW TO WORK.
	PTUnknown
)

func init() {
	if PTUnknown > 0x7e {
		panic(
			"PTUnknown (and therefore PropertyType) exceeds 0x7e. This conflicts " +
				"with serialize.WriteProperty's use of the high bit to indicate " +
				"NoIndex and/or \"impl/memory\".increment's ability to guarantee " +
				"incrementability.")
	}
}

// Property is a value plus an indicator of whether the value should be
// indexed. Name and Multiple are stored in the PropertyMap object.
type Property struct {
	value        interface{}
	indexSetting IndexSetting
	propType     PropertyType
}

// MkProperty makes a new indexed* Property and returns it. If val is an
// invalid value, this panics (so don't do it). If you want to handle the error
// normally, use SetValue(..., ShouldIndex) instead.
//
// *indexed if val is not an unindexable type like []byte.
func MkProperty(val interface{}) Property {
	ret := Property{}
	if err := ret.SetValue(val, ShouldIndex); err != nil {
		panic(err)
	}
	return ret
}

// MkPropertyNI makes a new Property (with noindex set to true), and returns
// it. If val is an invalid value, this panics (so don't do it). If you want to
// handle the error normally, use SetValue(..., NoIndex) instead.
func MkPropertyNI(val interface{}) Property {
	ret := Property{}
	if err := ret.SetValue(val, NoIndex); err != nil {
		panic(err)
	}
	return ret
}

// PropertyTypeOf returns the PT* type of the given Property-compatible
// value v. If checkValid is true, this method will also ensure that time.Time
// and GeoPoint have valid values.
func PropertyTypeOf(v interface{}, checkValid bool) (PropertyType, error) {
	switch x := v.(type) {
	case nil:
		return PTNull, nil
	case int64:
		return PTInt, nil
	case float64:
		return PTFloat, nil
	case bool:
		return PTBool, nil
	case []byte:
		return PTBytes, nil
	case blobstore.Key:
		return PTBlobKey, nil
	case string:
		return PTString, nil
	case *Key:
		// TODO(riannucci): Check key for validity in its own namespace?
		return PTKey, nil
	case time.Time:
		err := error(nil)
		if checkValid && (x.Before(minTime) || x.After(maxTime)) {
			err = errors.New("time value out of range")
		}
		if checkValid && !timeLocationIsUTC(x.Location()) {
			err = fmt.Errorf("time value has wrong Location: %v", x.Location())
		}
		return PTTime, err
	case GeoPoint:
		err := error(nil)
		if checkValid && !x.Valid() {
			err = errors.New("invalid GeoPoint value")
		}
		return PTGeoPoint, err
	default:
		return PTUnknown, fmt.Errorf("gae: Property has bad type %T", v)
	}
}

// timeLocationIsUTC tests if two time.Location are equal.
//
// This is tricky using the standard time API, as time is implicitly normalized
// to UTC and all equality checks are performed relative to that normalized
// time. To compensate, we instantiate two new time.Time using the respective
// Locations.
func timeLocationIsUTC(l *time.Location) bool {
	return time.Date(1970, 1, 1, 0, 0, 0, 0, l).Equal(utcTestTime)
}

// UpconvertUnderlyingType takes an object o, and attempts to convert it to
// its native datastore-compatible type. e.g. int16 will convert to int64, and
// `type Foo string` will convert to `string`.
func UpconvertUnderlyingType(o interface{}) interface{} {
	if o == nil {
		return o
	}

	v := reflect.ValueOf(o)
	t := v.Type()
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		o = v.Int()
	case reflect.Bool:
		o = v.Bool()
	case reflect.String:
		if t != typeOfBSKey {
			o = v.String()
		}
	case reflect.Float32, reflect.Float64:
		o = v.Float()
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			o = v.Bytes()
		}
	case reflect.Struct:
		if t == typeOfTime {
			// time in a Property can only hold microseconds
			tim := v.Interface().(time.Time)
			if !tim.IsZero() {
				o = v.Interface().(time.Time).Round(time.Microsecond)
			}
		}
	}
	return o
}

// Value returns the current value held by this property. It's guaranteed to
// be a valid value type (i.e. `p.SetValue(p.Value(), true)` will never return
// an error).
func (p *Property) Value() interface{} { return p.value }

// IndexSetting says whether or not the datastore should create indicies for
// this value.
func (p *Property) IndexSetting() IndexSetting { return p.indexSetting }

// Type is the PT* type of the data contained in Value().
func (p *Property) Type() PropertyType { return p.propType }

// SetValue sets the Value field of a Property, and ensures that its value
// conforms to the permissible types. That way, you're guaranteed that if you
// have a Property, its value is valid.
//
// value is the property value. The valid types are:
//	- int64
//	- time.Time
//	- bool
//	- string
//    (only the first 1500 bytes is indexable)
//	- []byte
//    (only the first 1500 bytes is indexable)
//	- blobstore.Key
//    (only the first 1500 bytes is indexable)
//	- float64
//	- *Key
//	- GeoPoint
// This set is smaller than the set of valid struct field types that the
// datastore can load and save. A Property Value cannot be a slice (apart
// from []byte); use multiple Properties instead. Also, a Value's type
// must be explicitly on the list above; it is not sufficient for the
// underlying type to be on that list. For example, a Value of "type
// myInt64 int64" is invalid. Smaller-width integers and floats are also
// invalid. Again, this is more restrictive than the set of valid struct
// field types.
//
// A value may also be the nil interface value; this is equivalent to
// Python's None but not directly representable by a Go struct. Loading
// a nil-valued property into a struct will set that field to the zero
// value.
func (p *Property) SetValue(value interface{}, is IndexSetting) (err error) {
	pt := PTNull
	if value != nil {
		value = UpconvertUnderlyingType(value)
		if pt, err = PropertyTypeOf(value, true); err != nil {
			return
		}
	}
	p.propType = pt
	p.value = value
	p.indexSetting = is
	return
}

// ForIndex gets a new Property with its value and type converted as if it were
// being stored in a datastore index. See the doc on PropertyType for more info.
func (p Property) ForIndex() Property {
	switch p.propType {
	case PTNull, PTInt, PTBool, PTString, PTFloat, PTGeoPoint, PTKey:
		return p

	case PTTime:
		v, _ := p.Project(PTInt)
		return Property{v, p.indexSetting, PTInt}

	case PTBytes, PTBlobKey:
		v, _ := p.Project(PTString)
		return Property{v, p.indexSetting, PTString}
	}
	panic(fmt.Errorf("unknown PropertyType: %s", p.propType))
}

// Project can be used to project a Property retrieved from a Projection query
// into a different datatype. For example, if you have a PTInt property, you
// could Project(PTTime) to convert it to a time.Time. The following conversions
// are supported:
//   PTXXX <-> PTXXX (i.e. identity)
//   PTInt <-> PTTime
//   PTString <-> PTBlobKey
//   PTString <-> PTBytes
//   PTNull <-> Anything
func (p *Property) Project(to PropertyType) (interface{}, error) {
	switch {
	case to == p.propType:
		return p.value, nil

	case to == PTInt && p.propType == PTTime:
		t := p.value.(time.Time)
		v := uint64(t.Unix())*1e6 + uint64(t.Nanosecond()/1e3)
		return int64(v), nil

	case to == PTTime && p.propType == PTInt:
		v := p.value.(int64)
		t := time.Unix(int64(v/1e6), int64((v%1e6)*1e3))
		if t.IsZero() {
			return time.Time{}, nil
		}
		return t.UTC(), nil

	case to == PTString && p.propType == PTBytes:
		return string(p.value.([]byte)), nil

	case to == PTString && p.propType == PTBlobKey:
		return string(p.value.(blobstore.Key)), nil

	case to == PTBytes && p.propType == PTString:
		return []byte(p.value.(string)), nil

	case to == PTBlobKey && p.propType == PTString:
		return blobstore.Key(p.value.(string)), nil

	case to == PTNull:
		return nil, nil

	case p.propType == PTNull:
		switch to {
		case PTInt:
			return int64(0), nil
		case PTTime:
			return time.Time{}, nil
		case PTBool:
			return false, nil
		case PTBytes:
			return []byte(nil), nil
		case PTString:
			return "", nil
		case PTFloat:
			return float64(0), nil
		case PTGeoPoint:
			return GeoPoint{}, nil
		case PTKey:
			return nil, nil
		case PTBlobKey:
			return blobstore.Key(""), nil
		}
		fallthrough
	default:
		return nil, fmt.Errorf("unable to project %s to %s", p.propType, to)
	}
}

func cmpVals(a, b interface{}, t PropertyType) int {
	cmpFloat := func(a, b float64) int {
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1
	}

	switch t {
	case PTNull:
		return 0

	case PTBool:
		a, b := a.(bool), b.(bool)
		if a == b {
			return 0
		}
		if a && !b {
			return 1
		}
		return -1

	case PTInt:
		a, b := a.(int64), b.(int64)
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1

	case PTString:
		a, b := a.(string), b.(string)
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1

	case PTFloat:
		return cmpFloat(a.(float64), b.(float64))

	case PTGeoPoint:
		a, b := a.(GeoPoint), b.(GeoPoint)
		cmp := cmpFloat(a.Lat, b.Lat)
		if cmp != 0 {
			return cmp
		}
		return cmpFloat(a.Lng, b.Lng)

	case PTKey:
		a, b := a.(*Key), b.(*Key)
		if a.Equal(b) {
			return 0
		}
		if b.Less(a) {
			return 1
		}
		return -1

	default:
		panic(fmt.Errorf("uncomparable type: %s", t))
	}
}

// Less returns true iff p would sort before other.
//
// This uses datastore's index rules for sorting (e.g.
// []byte("hello") == "hello")
func (p *Property) Less(other *Property) bool {
	if p.indexSetting && !other.indexSetting {
		return true
	} else if !p.indexSetting && other.indexSetting {
		return false
	}
	a, b := p.ForIndex(), other.ForIndex()
	cmp := int(a.propType) - int(b.propType)
	if cmp < 0 {
		return true
	} else if cmp > 0 {
		return false
	}
	return cmpVals(a.value, b.value, a.propType) < 0
}

// Equal returns true iff p and other have identical index representations.
//
// This uses datastore's index rules for sorting (e.g.
// []byte("hello") == "hello")
func (p *Property) Equal(other *Property) bool {
	ret := p.indexSetting == other.indexSetting
	if ret {
		a, b := p.ForIndex(), other.ForIndex()
		ret = a.propType == b.propType && cmpVals(a.value, b.value, a.propType) == 0
	}
	return ret
}

// GQL returns a correctly formatted Cloud Datastore GQL literal which
// is valid for a comparison value in the `WHERE` clause.
//
// The flavor of GQL that this emits is defined here:
//   https://cloud.google.com/datastore/docs/apis/gql/gql_reference
//
// NOTE: GeoPoint values are emitted with speculated future syntax. There is
// currently no syntax for literal GeoPoint values.
func (p *Property) GQL() string {
	switch p.propType {
	case PTNull:
		return "NULL"

	case PTInt, PTFloat, PTBool:
		return fmt.Sprint(p.value)

	case PTString:
		return gqlQuoteString(p.value.(string))

	case PTBytes:
		return fmt.Sprintf("BLOB(%q)",
			base64.URLEncoding.EncodeToString(p.value.([]byte)))

	case PTBlobKey:
		return fmt.Sprintf("BLOBKEY(%s)", gqlQuoteString(
			string(p.value.(blobstore.Key))))

	case PTKey:
		return p.value.(*Key).GQL()

	case PTTime:
		return fmt.Sprintf("DATETIME(%s)", p.value.(time.Time).Format(time.RFC3339Nano))

	case PTGeoPoint:
		// note that cloud SQL doesn't support this yet, but take a good guess at
		// it.
		v := p.value.(GeoPoint)
		return fmt.Sprintf("GEOPOINT(%v, %v)", v.Lat, v.Lng)
	}
	panic(fmt.Errorf("bad type: %s", p.propType))
}

// PropertySlice is a slice of Properties. It implements sort.Interface.
type PropertySlice []Property

func (s PropertySlice) Len() int           { return len(s) }
func (s PropertySlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s PropertySlice) Less(i, j int) bool { return s[i].Less(&s[j]) }

// MetaGetter is a subinterface of PropertyLoadSaver, but is also used to
// abstract the meta argument for RawInterface.GetMulti.
type MetaGetter interface {
	// GetMeta will get information about the field which has the struct tag in
	// the form of `gae:"$<key>[,<default>]?"`.
	//
	// Supported metadata types are:
	//   int64  - may have default (ascii encoded base-10)
	//   string - may have default
	//   Toggle - MUST have default ("true" or "false")
	//   *Key    - NO default allowed
	//
	// Struct fields of type Toggle (which is an Auto/On/Off) require you to
	// specify a value of 'true' or 'false' for the default value of the struct
	// tag, and GetMeta will return the combined value as a regular boolean true
	// or false value.
	// Example:
	//   type MyStruct struct {
	//     CoolField int64 `gae:"$id,1"`
	//   }
	//   val, err := helper.GetPLS(&MyStruct{}).GetMeta("id")
	//   // val == 1
	//   // err == nil
	//
	//   val, err := helper.GetPLS(&MyStruct{10}).GetMeta("id")
	//   // val == 10
	//   // err == nil
	//
	//   type MyStruct struct {
	//     TFlag Toggle `gae:"$flag1,true"`  // defaults to true
	//     FFlag Toggle `gae:"$flag2,false"` // defaults to false
	//     // BadFlag  Toggle `gae:"$flag3"` // ILLEGAL
	//   }
	GetMeta(key string) (interface{}, error)

	// GetMetaDefault is GetMeta, but with a default.
	//
	// If the metadata key is not available, or its type doesn't equal the
	// homogenized type of dflt, then dflt will be returned.
	//
	// Type homogenization:
	//   signed integer types -> int64
	//   bool                 -> Toggle fields (bool)
	//
	// Example:
	//   pls.GetMetaDefault("foo", 100).(int64)
	GetMetaDefault(key string, dflt interface{}) interface{}
}

// PropertyLoadSaver may be implemented by a user type, and Interface will
// use this interface to serialize the type instead of trying to automatically
// create a serialization codec for it with helper.GetPLS.
type PropertyLoadSaver interface {
	// Load takes the values from the given map and attempts to save them into
	// the underlying object (usually a struct or a PropertyMap). If a fatal
	// error occurs, it's returned via error. If non-fatal conversion errors
	// occur, error will be a MultiError containing one or more ErrFieldMismatch
	// objects.
	Load(PropertyMap) error

	// Save returns the current property as a PropertyMap. if withMeta is true,
	// then the PropertyMap contains all the metadata (e.g. '$meta' fields)
	// which was held by this PropertyLoadSaver.
	Save(withMeta bool) (PropertyMap, error)

	MetaGetter

	// GetAllMeta returns a PropertyMap with all of the metadata in this
	// PropertyLoadSaver. If a metadata field has an error during serialization,
	// it is skipped.
	GetAllMeta() PropertyMap

	// SetMeta allows you to set the current value of the meta-keyed field.
	SetMeta(key string, val interface{}) error

	// Problem indicates that this PLS has a fatal problem. Usually this is
	// set when the underlying struct has recursion, invalid field types, nested
	// slices, etc.
	Problem() error
}

// PropertyMap represents the contents of a datastore entity in a generic way.
// It maps from property name to a list of property values which correspond to
// that property name. It is the spiritual successor to PropertyList from the
// original SDK.
//
// PropertyMap may contain "meta" values, which are keyed with a '$' prefix.
// Technically the datastore allows arbitrary property names, but all of the
// SDKs go out of their way to try to make all property names valid programming
// language tokens. Special values must correspond to a single Property...
// corresponding to 0 is equivalent to unset, and corresponding to >1 is an
// error. So:
//
//   {
//     "$id": {MkProperty(1)}, // GetProperty("id") -> 1, nil
//     "$foo": {}, // GetProperty("foo") -> nil, ErrMetaFieldUnset
//     // GetProperty("bar") -> nil, ErrMetaFieldUnset
//     "$meep": {
//       MkProperty("hi"),
//       MkProperty("there")}, // GetProperty("meep") -> nil, error!
//   }
//
// Additionally, Save returns a copy of the map with the meta keys omitted (e.g.
// these keys are not going to be serialized to the datastore).
type PropertyMap map[string][]Property

var _ PropertyLoadSaver = PropertyMap(nil)

// Load implements PropertyLoadSaver.Load
func (pm PropertyMap) Load(props PropertyMap) error {
	for k, v := range props {
		pm[k] = append(pm[k], v...)
	}
	return nil
}

// Save implements PropertyLoadSaver.Save by returning a copy of the
// current map data.
func (pm PropertyMap) Save(withMeta bool) (PropertyMap, error) {
	if len(pm) == 0 {
		return PropertyMap{}, nil
	}
	ret := make(PropertyMap, len(pm))
	for k, v := range pm {
		if withMeta || !isMetaKey(k) {
			ret[k] = append(ret[k], v...)
		}
	}
	return ret, nil
}

// GetMeta implements PropertyLoadSaver.GetMeta, and returns the current value
// associated with the metadata key. It may return ErrMetaFieldUnset if the
// key doesn't exist.
func (pm PropertyMap) GetMeta(key string) (interface{}, error) {
	v, ok := pm["$"+key]
	if !ok || len(v) == 0 {
		return nil, ErrMetaFieldUnset
	}
	if len(v) > 1 {
		return nil, errors.New("gae: too many values for Meta key")
	}
	return v[0].Value(), nil
}

// GetAllMeta implements PropertyLoadSaver.GetAllMeta.
func (pm PropertyMap) GetAllMeta() PropertyMap {
	ret := make(PropertyMap, 8)
	for k, v := range pm {
		if isMetaKey(k) {
			newV := make([]Property, len(v))
			copy(newV, v)
			ret[k] = newV
		}
	}
	return ret
}

// GetMetaDefault is the implementation of PropertyLoadSaver.GetMetaDefault.
func (pm PropertyMap) GetMetaDefault(key string, dflt interface{}) interface{} {
	return GetMetaDefaultImpl(pm.GetMeta, key, dflt)
}

// SetMeta implements PropertyLoadSaver.SetMeta. It will only return an error
// if `val` has an invalid type (e.g. not one supported by Property).
func (pm PropertyMap) SetMeta(key string, val interface{}) error {
	prop := Property{}
	if err := prop.SetValue(val, NoIndex); err != nil {
		return err
	}
	pm["$"+key] = []Property{prop}
	return nil
}

// Problem implements PropertyLoadSaver.Problem. It ALWAYS returns nil.
func (pm PropertyMap) Problem() error {
	return nil
}

func isMetaKey(k string) bool {
	// empty counts as a metakey since it's not a valid data key, but it's
	// not really a valid metakey either.
	return k == "" || k[0] == '$'
}

// GetMetaDefaultImpl is the implementation of PropertyLoadSaver.GetMetaDefault.
//
// It takes the normal GetMeta function, the key and the default, and returns
// the value according to PropertyLoadSaver.GetMetaDefault.
func GetMetaDefaultImpl(gm func(string) (interface{}, error), key string, dflt interface{}) interface{} {
	dflt = UpconvertUnderlyingType(dflt)
	cur, err := gm(key)
	if err != nil {
		return dflt
	}
	if dflt != nil && reflect.TypeOf(cur) != reflect.TypeOf(dflt) {
		return dflt
	}
	return cur
}
