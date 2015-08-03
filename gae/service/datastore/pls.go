// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"reflect"
)

// GetPLS resolves obj into a PropertyLoadSaver.
//
// obj must be a non-nil pointer to a struct of some sort.
//
// By default, exported fields will be serialized to/from the datastore. If the
// field is not exported, it will be skipped by the serialization routines.
//
// If a field is of a non-supported type (see Property for the list of supported
// property types), the resulting PropertyLoadSaver will have a non-nil
// Problem(). Other problems include duplicate field names (due to tagging),
// recursively defined structs, nested structures with multiple slices (e.g.
// slices of slices, either directly `[][]type` or indirectly `[]Embedded` where
// Embedded contains a slice.)
//
// GetPLS supports the following struct tag syntax:
//   `gae:"fieldName[,noindex]"` -- an alternate fieldname for an exportable
//	    field.  When the struct is serialized or deserialized, fieldName will be
//      associated with the struct field instead of the field's Go name. This is
//      useful when writing Go code which interfaces with appengine code written
//      in other languages (like python) which use lowercase as their default
//		  datastore field names.
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
//        - Key
//        - int64
//        - string
//        - Toggle (GetMeta and SetMeta treat the field as if it were bool)
//      Additionally, int64, string and Toggle allow setting a default value
//      in the struct field tag (the "<value>" portion).
//
//      Only exported fields allow SetMeta, but all fields of appropriate type
//      allow tagged defaults. See Examples.
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
//     Parent Key `gae:"$parent"`
//
//     // 'Lines' will serialized to the datastore in the field 'Lines'
//     Lines []string
//   }
func GetPLS(obj interface{}) PropertyLoadSaver {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return &structPLS{c: &structCodec{problem: ErrInvalidEntityType}}
	}
	if v.IsNil() {
		return &structPLS{c: &structCodec{problem: ErrInvalidEntityType}}
	}
	v = v.Elem()
	c := getCodec(v.Type())
	return &structPLS{v, c}
}

func getCodec(structType reflect.Type) *structCodec {
	structCodecsMutex.RLock()
	c, ok := structCodecs[structType]
	structCodecsMutex.RUnlock()
	if ok {
		return c
	}

	structCodecsMutex.Lock()
	defer structCodecsMutex.Unlock()
	return getStructCodecLocked(structType)
}
