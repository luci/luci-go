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
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"go.chromium.org/gae/service/blobstore"
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
	// value is the Property's value. It is stored in an internal, opaque type and
	// should not be directly exported to consumers. Rather, it can be accessed
	// in its original value via Value().
	//
	// Specifically:
	//	- []byte- and string-based values are stored in a bytesByteSequence and
	//	  stringByteSequence respectively.
	value interface{}

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

// RoundTime rounds a time.Time to microseconds, which is the (undocumented)
// way that the AppEngine SDK stores it.
func RoundTime(t time.Time) time.Time {
	return t.Round(time.Microsecond)
}

// TimeToInt converts a time value to a datastore-appropraite integer value.
//
// This method truncates the time to microseconds and drops the timezone,
// because that's the (undocumented) way that the appengine SDK does it.
func TimeToInt(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}

	t = RoundTime(t)
	return t.Unix()*1e6 + int64(t.Nanosecond()/1e3)
}

// IntToTime converts a datastore time integer into its time.Time value.
func IntToTime(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return RoundTime(time.Unix(int64(v/1e6), int64((v%1e6)*1e3))).UTC()
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
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		o = int64(v.Uint())
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
			tim := v.Interface().(time.Time)
			if !tim.IsZero() {
				o = RoundTime(v.Interface().(time.Time))
			}
		}
	}

	switch t {
	case typeOfKey:
		if v.IsNil() {
			return nil
		}
	}
	return o
}

func (Property) isAPropertyData() {}

func (p Property) estimateSize() int64 { return p.EstimateSize() }

// Slice implements the PropertyData interface.
func (p Property) Slice() PropertySlice { return PropertySlice{p} }

// Clone implements the PropertyData interface.
func (p Property) Clone() PropertyData { return p }

func (p Property) String() string {
	switch p.propType {
	case PTString, PTBlobKey:
		return fmt.Sprintf("%s(%q)", p.propType, p.Value())
	case PTBytes:
		return fmt.Sprintf("%s(%#x)", p.propType, p.Value())
	default:
		return fmt.Sprintf("%s(%v)", p.propType, p.Value())
	}
}

// Value returns the current value held by this property. It's guaranteed to
// be a valid value type (i.e. `p.SetValue(p.Value(), true)` will never return
// an error).
func (p *Property) Value() interface{} {
	switch p.propType {
	case PTBytes:
		return p.value.(byteSequence).bytes()
	case PTString:
		return p.value.(byteSequence).string()
	case PTBlobKey:
		return blobstore.Key(p.value.(byteSequence).string())
	default:
		return p.value
	}
}

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

	// Convert value to internal Property storage type.
	switch t := value.(type) {
	case string:
		value = stringByteSequence(t)
	case blobstore.Key:
		value = stringByteSequence(t)
	case []byte:
		value = bytesByteSequence(t)
	case time.Time:
		value = RoundTime(t)
	}

	p.propType = pt
	p.value = value
	p.indexSetting = is
	return
}

// IndexTypeAndValue returns the type and value of the Property as it would
// show up in a datastore index.
//
// This is used to operate on the Property as it would be stored in a datastore
// index, specifically for serialization and comparison.
//
// The returned type will be the PropertyType used in the index. The returned
// value will be one of:
//	- bool
//	- int64
//	- float64
//	- string
//	- []byte
//	- GeoPoint
//	- *Key
func (p Property) IndexTypeAndValue() (PropertyType, interface{}) {
	switch t := p.propType; t {
	case PTNull, PTInt, PTBool, PTFloat, PTGeoPoint, PTKey:
		return t, p.Value()

	case PTTime:
		return PTInt, TimeToInt(p.value.(time.Time))

	case PTString, PTBytes, PTBlobKey:
		return PTString, p.value.(byteSequence).value()

	default:
		panic(fmt.Errorf("unknown PropertyType: %s", t))
	}
}

// Project can be used to project a Property retrieved from a Projection query
// into a different datatype. For example, if you have a PTInt property, you
// could Project(PTTime) to convert it to a time.Time. The following conversions
// are supported:
//   PTString <-> PTBlobKey
//   PTString <-> PTBytes
//   PTXXX <-> PTXXX (i.e. identity)
//   PTInt <-> PTTime
//   PTNull <-> Anything
func (p *Property) Project(to PropertyType) (interface{}, error) {
	if to == PTNull {
		return nil, nil
	}

	pt, v := p.propType, p.value
	switch pt {
	case PTBytes, PTString, PTBlobKey:
		v := v.(byteSequence)
		switch to {
		case PTBytes:
			return v.bytes(), nil
		case PTString:
			return v.string(), nil
		case PTBlobKey:
			return blobstore.Key(v.string()), nil
		}

	case PTTime:
		switch to {
		case PTInt:
			return TimeToInt(v.(time.Time)), nil
		case PTTime:
			return v, nil
		}

	case PTInt:
		switch to {
		case PTInt:
			return v, nil
		case PTTime:
			return IntToTime(v.(int64)), nil
		}

	case to:
		return v, nil

	case PTNull:
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
	}
	return nil, fmt.Errorf("unable to project %s to %s", pt, to)
}

func cmpFloat(a, b float64) int {
	if a == b {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}

// Less returns true iff p would sort before other.
//
// This uses datastore's index rules for sorting (see GetIndexTypeAndValue).
func (p *Property) Less(other *Property) bool {
	return p.Compare(other) < 0
}

// Equal returns true iff p and other have identical index representations.
//
// This uses datastore's index rules for sorting (see GetIndexTypeAndValue).
func (p *Property) Equal(other *Property) bool {
	return p.Compare(other) == 0
}

// Compare compares this Property to another, returning a trinary value
// indicating where it would sort relative to the other in datastore.
//
// It returns:
//	<0 if the Property would sort before `other`.
//	>0 if the Property would after before `other`.
//	0 if the Property equals `other`.
//
// This uses datastore's index rules for sorting (see GetIndexTypeAndValue).
func (p *Property) Compare(other *Property) int {
	if p.indexSetting && !other.indexSetting {
		return 1
	} else if !p.indexSetting && other.indexSetting {
		return -1
	}

	at, av := p.IndexTypeAndValue()
	bt, bv := other.IndexTypeAndValue()
	if cmp := int(at) - int(bt); cmp != 0 {
		return cmp
	}

	switch t := at; t {
	case PTNull:
		return 0

	case PTBool:
		a, b := av.(bool), bv.(bool)
		if a == b {
			return 0
		}
		if a && !b {
			return 1
		}
		return -1

	case PTInt:
		a, b := av.(int64), bv.(int64)
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1

	case PTString:
		return cmpByteSequence(p.value.(byteSequence), other.value.(byteSequence))

	case PTFloat:
		return cmpFloat(av.(float64), bv.(float64))

	case PTGeoPoint:
		a, b := av.(GeoPoint), bv.(GeoPoint)
		cmp := cmpFloat(a.Lat, b.Lat)
		if cmp != 0 {
			return cmp
		}
		return cmpFloat(a.Lng, b.Lng)

	case PTKey:
		a, b := av.(*Key), bv.(*Key)
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

// EstimateSize estimates the amount of space that this Property would consume
// if it were committed as part of an entity in the real production datastore.
//
// It uses https://cloud.google.com/appengine/articles/storage_breakdown?csw=1
// as a guide for these values.
func (p *Property) EstimateSize() int64 {
	switch p.Type() {
	case PTNull:
		return 1
	case PTBool:
		return 1 + 4
	case PTInt, PTTime, PTFloat:
		return 1 + 8
	case PTGeoPoint:
		return 1 + (8 * 2)
	case PTString:
		return 1 + int64(len(p.Value().(string)))
	case PTBlobKey:
		return 1 + int64(len(p.Value().(blobstore.Key)))
	case PTBytes:
		return 1 + int64(len(p.Value().([]byte)))
	case PTKey:
		return 1 + p.Value().(*Key).EstimateSize()
	}
	panic(fmt.Errorf("Unknown property type: %s", p.Type().String()))
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
	v := p.Value()
	switch p.propType {
	case PTNull:
		return "NULL"

	case PTInt, PTFloat, PTBool:
		return fmt.Sprint(v)

	case PTString:
		return gqlQuoteString(v.(string))

	case PTBytes:
		return fmt.Sprintf("BLOB(%q)",
			base64.URLEncoding.EncodeToString(v.([]byte)))

	case PTBlobKey:
		return fmt.Sprintf("BLOBKEY(%s)", gqlQuoteString(
			string(v.(blobstore.Key))))

	case PTKey:
		return v.(*Key).GQL()

	case PTTime:
		return fmt.Sprintf("DATETIME(%s)", v.(time.Time).Format(time.RFC3339Nano))

	case PTGeoPoint:
		// note that cloud SQL doesn't support this yet, but take a good guess at
		// it.
		v := v.(GeoPoint)
		return fmt.Sprintf("GEOPOINT(%v, %v)", v.Lat, v.Lng)
	}
	panic(fmt.Errorf("bad type: %s", p.propType))
}

// PropertySlice is a slice of Properties. It implements sort.Interface.
//
// PropertySlice holds multiple Properties. Writing a PropertySlice to datastore
// implicitly marks the property as "multiple", even if it only has one element.
type PropertySlice []Property

func (PropertySlice) isAPropertyData() {}

// Clone implements the PropertyData interface.
func (s PropertySlice) Clone() PropertyData { return s.Slice() }

// Slice implements the PropertyData interface.
func (s PropertySlice) Slice() PropertySlice {
	if len(s) == 0 {
		return nil
	}
	return append(make(PropertySlice, 0, len(s)), s...)
}

func (s PropertySlice) estimateSize() (v int64) {
	for _, prop := range s {
		// Use the public one so we don't have to copy.
		v += prop.estimateSize()
	}
	return
}

func (s PropertySlice) Len() int           { return len(s) }
func (s PropertySlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s PropertySlice) Less(i, j int) bool { return s[i].Less(&s[j]) }

// MetaGetter is a subinterface of PropertyLoadSaver, but is also used to
// abstract the meta argument for RawInterface.GetMulti.
type MetaGetter interface {
	// GetMeta will get information about the field which has the struct tag in
	// the form of `gae:"$<key>[,<default>]?"`.
	//
	// It returns the value, if any, and true iff the value was retrieved.
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
	GetMeta(key string) (interface{}, bool)
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
}

// MetaGetterSetter is the subset of PropertyLoadSaver which pertains to
// getting and saving metadata.
//
// A *struct may implement this interface to provide metadata which is
// supplimental to the variety described by GetPLS. For example, this could be
// used to implement a parsed-out $kind or $id.
type MetaGetterSetter interface {
	MetaGetter

	// GetAllMeta returns a PropertyMap with all of the metadata in this
	// MetaGetterSetter. If a metadata field has an error during serialization,
	// it is skipped.
	//
	// If a *struct is implementing this, then it only needs to return the
	// metadata fields which would be returned by its GetMeta implementation, and
	// the `GetPLS` implementation will add any statically-defined metadata
	// fields. So if GetMeta provides $id, but there's a simple tagged field for
	// $kind, this method is only expected to return a PropertyMap with "$id".
	GetAllMeta() PropertyMap

	// SetMeta allows you to set the current value of the meta-keyed field.
	// It returns true iff the field was set.
	SetMeta(key string, val interface{}) bool
}

// PropertyData is an interface implemented by Property and PropertySlice to
// identify themselves as valid PropertyMap values.
type PropertyData interface {
	// isAPropertyData is a tag that forces PropertyData implementations to only
	// be supplied by this package.
	isAPropertyData()

	// Slice returns a PropertySlice representation of this PropertyData.
	//
	// The returned PropertySlice is a clone of the original data. Consequently,
	// Consequently, Property-modifying methods such as SetValue should NOT be
	// called on the results.
	Slice() PropertySlice

	// estimateSize estimates the aggregate size of the property data.
	estimateSize() int64

	// Clone creates a duplicate copy of this PropertyData.
	Clone() PropertyData
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
type PropertyMap map[string]PropertyData

var _ PropertyLoadSaver = PropertyMap(nil)

// Load implements PropertyLoadSaver.Load
func (pm PropertyMap) Load(props PropertyMap) error {
	for k, v := range props {
		pm[k] = v.Clone()
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
			ret[k] = v.Clone()
		}
	}
	return ret, nil
}

// GetMeta implements PropertyLoadSaver.GetMeta, and returns the current value
// associated with the metadata key.
func (pm PropertyMap) GetMeta(key string) (interface{}, bool) {
	pslice := pm.Slice("$" + key)
	if len(pslice) > 0 {
		return pslice[0].Value(), true
	}
	return nil, false
}

// GetAllMeta implements PropertyLoadSaver.GetAllMeta.
func (pm PropertyMap) GetAllMeta() PropertyMap {
	ret := make(PropertyMap, 8)
	for k, v := range pm {
		if isMetaKey(k) {
			ret[k] = v.Clone()
		}
	}
	return ret
}

// SetMeta implements PropertyLoadSaver.SetMeta. It will only return an error
// if `val` has an invalid type (e.g. not one supported by Property).
func (pm PropertyMap) SetMeta(key string, val interface{}) bool {
	prop := Property{}
	if err := prop.SetValue(val, NoIndex); err != nil {
		return false
	}
	pm["$"+key] = prop
	return true
}

// Problem implements PropertyLoadSaver.Problem. It ALWAYS returns nil.
func (pm PropertyMap) Problem() error {
	return nil
}

// Slice returns a PropertySlice for the given key
//
// If the value associated with that key is nil, an empty slice will be
// returned. If the value is single Property, a slice of size 1 with that
// Property in it will be returned.
func (pm PropertyMap) Slice(key string) PropertySlice {
	if pdata := pm[key]; pdata != nil {
		return pdata.Slice()
	}
	return nil
}

// EstimateSize estimates the size that it would take to encode this PropertyMap
// in the production Appengine datastore. The calculation excludes metadata
// fields in the map.
//
// It uses https://cloud.google.com/appengine/articles/storage_breakdown?csw=1
// as a guide for sizes.
func (pm PropertyMap) EstimateSize() int64 {
	ret := int64(0)
	for k, vals := range pm {
		if !isMetaKey(k) {
			ret += int64(len(k))
			ret += vals.estimateSize()
		}
	}
	return ret
}

func isMetaKey(k string) bool {
	// empty counts as a metakey since it's not a valid data key, but it's
	// not really a valid metakey either.
	return k == "" || k[0] == '$'
}

// GetMetaDefault is a helper for GetMeta, allowing a default value.
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
func GetMetaDefault(getter MetaGetter, key string, dflt interface{}) interface{} {
	dflt = UpconvertUnderlyingType(dflt)
	cur, ok := getter.GetMeta(key)
	if !ok || (dflt != nil && reflect.TypeOf(cur) != reflect.TypeOf(dflt)) {
		return dflt
	}
	return cur
}

// byteSequence is a generic interface for an object that can be represented as
// a sequence of bytes. Its implementations are used internally by Property to
// enable zero-copy conversion and comparisons between byte sequence types.
type byteSequence interface {
	// len returns the number of bytes in the sequence.
	len() int
	// get returns the byte at the specified index.
	get(int) byte
	// value returns the sequence's primitive type.
	value() interface{}
	// string returns the sequence as a string (may cause a copy if not native).
	string() string
	// bytes returns the sequence as a []byte (may cause a copy if not native).
	bytes() []byte
	// fastCmp is an implementation-specific comparison method. If it returns
	// true in its second return value, the comparison was performed and its
	// result is in the first return value. Otherwise, the fast comparison could
	// not be performed.
	fastCmp(o byteSequence) (int, bool)
}

func cmpByteSequence(a, b byteSequence) int {
	if v, ok := a.fastCmp(b); ok {
		return v
	}

	// Byte-by-byte "slow" comparison.
	ln := a.len()
	if bln := b.len(); bln < ln {
		ln = bln
	}

	for i := 0; i < ln; i++ {
		av, bv := a.get(i), b.get(i)
		switch {
		case av < bv:
			return -1
		case av > bv:
			return 1
		}
	}

	return a.len() - b.len()
}

// bytesByteSequence is a byteSequence implementation for a byte slice.
type bytesByteSequence []byte

func (s bytesByteSequence) len() int           { return len(s) }
func (s bytesByteSequence) get(i int) byte     { return s[i] }
func (s bytesByteSequence) value() interface{} { return []byte(s) }
func (s bytesByteSequence) string() string     { return string(s) }
func (s bytesByteSequence) bytes() []byte      { return []byte(s) }
func (s bytesByteSequence) fastCmp(o byteSequence) (int, bool) {
	if t, ok := o.(bytesByteSequence); ok {
		return bytes.Compare([]byte(s), []byte(t)), true
	}
	return 0, false
}

// stringByteSequence is a byteSequence implementation for a string.
type stringByteSequence string

func (s stringByteSequence) len() int           { return len(s) }
func (s stringByteSequence) get(i int) byte     { return s[i] }
func (s stringByteSequence) value() interface{} { return string(s) }
func (s stringByteSequence) string() string     { return string(s) }
func (s stringByteSequence) bytes() []byte      { return []byte(s) }
func (s stringByteSequence) fastCmp(o byteSequence) (int, bool) {
	if t, ok := o.(stringByteSequence); ok {
		// This pattern is used for string comparison in strings.Compare.
		if string(s) == string(t) {
			return 0, true
		}
		if string(s) < string(t) {
			return -1, true
		}
		return 1, true
	}
	return 0, false
}
