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
	"fmt"
	"reflect"
)

// GetPLS resolves obj into default struct PropertyLoadSaver and
// MetaGetterSetter implementation.
//
// obj must be a non-nil pointer to a struct of some sort.
//
// By default, exported fields will be serialized to/from the datastore. If the
// field is not exported, it will be skipped by the serialization routines.
//
// If a field is of a non-supported type (see Property for the list of supported
// property types), this function will panic. Other problems include duplicate
// field names (due to tagging), recursively defined structs, nested structures
// with multiple slices (e.g.  slices of slices, either directly `[][]type` or
// indirectly `[]Embedded` where Embedded contains a slice.)
//
// The following field types are supported:
//   * int64, int32, int16, int8, int
//   * uint32, uint16, uint8, byte
//   * float64, float32
//   * string
//   * []byte
//   * bool
//   * time.Time
//   * GeoPoint
//   * *Key
//   * any Type whose underlying type is one of the above types
//   * Types which implement PropertyConverter on (*Type)
//   * A struct composed of the above types (except for nested slices)
//   * A slice of any of the above types
//
// GetPLS supports the following struct tag syntax:
//   `gae:"fieldName[,noindex]"` -- an alternate fieldname for an exportable
//      field.  When the struct is serialized or deserialized, fieldName will be
//      associated with the struct field instead of the field's Go name. This is
//      useful when writing Go code which interfaces with appengine code written
//      in other languages (like python) which use lowercase as their default
//      datastore field names.
//
//      A fieldName of "-" means that gae will ignore the field for all
//      serialization/deserialization.
//
//      if noindex is specified, then this field will not be indexed in the
//      datastore, even if it was an otherwise indexable type. If fieldName is
//      blank, and noindex is specifed, then fieldName will default to the
//      field's actual name. Note that by default, all fields (with indexable
//      types) are indexed.
//
//   `gae:"$metaKey[,<value>]` -- indicates a field is metadata. Metadata
//      can be used to control filter behavior, or to store key data when using
//      the Interface.KeyForObj* methods. The supported field types are:
//        - *Key
//        - int64, int32, int16, int8, uint32, uint16, uint8, byte
//        - string
//        - Toggle (GetMeta and SetMeta treat the field as if it were bool)
//        - Any type which implements PropertyConverter
//      Additionally, numeric, string and Toggle types allow setting a default
//      value in the struct field tag (the "<value>" portion).
//
//      Only exported fields allow SetMeta, but all fields of appropriate type
//      allow tagged defaults for use with GetMeta. See Examples.
//
//   `gae:"[-],extra"` -- indicates that any extra, unrecognized or mismatched
//      property types (type in datastore doesn't match your struct's field
//      type) should be loaded into and saved from this field. The precise type
//      of the field must be PropertyMap. This form allows you to control the
//      behavior of reads and writes when your schema changes, or to implement
//      something like ndb.Expando with a mix of structured and unstructured
//      fields.
//
//      If the `-` is present, then datastore write operations will not put
//      elements of this map into the datastore.
//
//      If the field is non-exported, then read operations from the datastore
//      will not populate the members of this map, but extra fields or
//      structural differences encountered when reading into this struct will be
//      silently ignored. This is useful if you want to just ignore old fields.
//
//      If there is a conflict between a field in the struct and a same-named
//      Property in the extra field, the field in the struct takes precedence.
//
//      Recursive structs are supported, but all extra properties go to the
//      topmost structure's Extra field. This is a bit non-intuitive, but the
//      implementation complexity was deemed not worth it, since that sort of
//      thing is generally only useful on schema changes, which should be
//      transient.
//
//      Examples:
//        // "black hole": ignore mismatches, ignore on write
//        _ PropertyMap `gae:"-,extra"
//
//        // "expando": full content is read/written
//        Expando PropertyMap `gae:",extra"
//
//        // "convert": content is read from datastore, but lost on writes. This
//        // is useful for doing conversions from an old schema to a new one,
//        // since you can retrieve the old data and populate it into new fields,
//        // for example. Probably should be used in conjunction with an
//        // implementation of the PropertyLoadSaver interface so that you can
//        // transparently upconvert to the new schema on load.
//        Convert PropertyMap `gae:"-,extra"
//
// Example "special" structure. This is supposed to be some sort of datastore
// singleton object.
//   struct secretFoo {
//     // _id and _kind are not exported, so setting their values will not be
//     // reflected by GetMeta.
//     _id   int64  `gae:"$id,1"`
//     _kind string `gae:"$kind,InternalFooSingleton"`
//
//     // Value is exported, so can be read and written by the PropertyLoadSaver,
//     // but secretFoo is shared with a python appengine module which has
//     // stored this field as 'value' instead of 'Value'.
//     Value int64  `gae:"value"`
//   }
//
// Example "normal" structure that you might use in a go-only appengine app.
//   struct User {
//     ID string `gae:"$id"`
//     // "kind" is automatically implied by the struct name: "User"
//     // "parent" is nil... Users are root entities
//
//     // 'Name' will serialized to the datastore in the field 'Name'
//     Name string
//   }
//
//   struct Comment {
//     ID int64 `gae:"$id"`
//     // "kind" is automatically implied by the struct name: "Comment"
//
//     // Parent will be enforced by the application to be a User key.
//     Parent *Key `gae:"$parent"`
//
//     // 'Lines' will serialized to the datastore in the field 'Lines'
//     Lines []string
//   }
//
// A pointer-to-struct may also implement MetaGetterSetter to provide more
// sophistocated metadata values. Explicitly defined fields (as shown above)
// always take precedence over fields manipulated by the MetaGetterSetter
// methods. So if your GetMeta handles "kind", but you explicitly have a
// $kind field, the $kind field will take precedence and your GetMeta
// implementation will not be called for "kind".
//
// A struct overloading any of the PropertyLoadSaver or MetaGetterSetter
// interfaces may evoke the default struct behavior by using GetPLS on itself.
// For example:
//
//   struct Special {
//     Name string
//
//     foo string
//   }
//
//   func (s *Special) Load(props PropertyMap) error {
//     if foo, ok := props["foo"]; ok && len(foo) == 1 {
//       s.foo = foo
//       delete(props, "foo")
//     }
//     return GetPLS(s).Load(props)
//   }
//
//   func (s *Special) Save(withMeta bool) (PropertyMap, error) {
//     props, err := GetPLS(s).Save(withMeta)
//     if err != nil {
//       return nil, err
//     }
//     props["foo"] = []Property{MkProperty(s.foo)}
//     return props, nil
//   }
//
//   func (s *Special) Problem() error {
//     return GetPLS(s).Problem()
//   }
//
// Additionally, any field ptr-to-type may implement the PropertyConverter
// interface to allow a single field to, for example, implement some alternate
// encoding (json, gzip), or even just serialize to/from a simple string field.
// This applies to normal fields, as well as metadata fields. It can be useful
// for storing struct '$id's which have multi-field meanings. For example, the
// Person struct below could be initialized in go as `&Person{Name{"Jane",
// "Doe"}}`, retaining Jane's name as manipulable Go fields. However, in the
// datastore, it would have a key of `/Person,"Jane|Doe"`, and loading the
// struct from the datastore as part of a Query, for example, would correctly
// populate Person.Name.First and Person.Name.Last.
//
//   type Name struct {
//     First string
//     Last string
//   }
//
//   func (n *Name) ToProperty() (Property, error) {
//     return fmt.Sprintf("%s|%s", n.First, n.Last)
//   }
//
//   func (n *Name) FromProperty(p Property) error {
//     // check p to be a PTString
//     // split on "|"
//     // assign to n.First, n.Last
//   }
//
//   type Person struct {
//     ID Name `gae:"$id"`
//   }
func GetPLS(obj interface{}) interface {
	PropertyLoadSaver
	MetaGetterSetter
} {
	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		panic(fmt.Errorf("cannot GetPLS(%T): failed to reflect", obj))
	}
	if v.IsNil() {
		panic(fmt.Errorf("cannot GetPLS(%T): pointer is nil", obj))
	}

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		if v.Kind() == reflect.Struct {
			s := structPLS{
				c: getCodec(v.Type()),
				o: v,
			}

			// If our object implements MetaGetterSetter, use this instead of the built-in
			// PLS MetaGetterSetter.
			if mgs, ok := obj.(MetaGetterSetter); ok {
				s.mgs = mgs
			}
			return &s
		}
	}
	panic(fmt.Errorf("cannot GetPLS(%T): not a pointer-to-struct", obj))
}

func getMGS(obj interface{}) MetaGetterSetter {
	if mgs, ok := obj.(MetaGetterSetter); ok {
		return mgs
	}
	return GetPLS(obj)
}

func getCodec(structType reflect.Type) *structCodec {
	structCodecsMutex.RLock()
	c, ok := structCodecs[structType]
	structCodecsMutex.RUnlock()
	if !ok {
		structCodecsMutex.Lock()
		defer structCodecsMutex.Unlock()
		c = getStructCodecLocked(structType)
	}
	if c.problem != nil {
		panic(c.problem)
	}
	return c
}
