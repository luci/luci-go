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
	"reflect"
	"time"

	"go.chromium.org/gae/service/blobstore"
)

var (
	typeOfBool              = reflect.TypeOf(true)
	typeOfBSKey             = reflect.TypeOf(blobstore.Key(""))
	typeOfCursorCB          = reflect.TypeOf(CursorCB(nil))
	typeOfGeoPoint          = reflect.TypeOf(GeoPoint{})
	typeOfInt64             = reflect.TypeOf(int64(0))
	typeOfKey               = reflect.TypeOf((*Key)(nil))
	typeOfPropertyConverter = reflect.TypeOf((*PropertyConverter)(nil)).Elem()
	typeOfPropertyLoadSaver = reflect.TypeOf((*PropertyLoadSaver)(nil)).Elem()
	typeOfMetaGetterSetter  = reflect.TypeOf((*MetaGetterSetter)(nil)).Elem()
	typeOfString            = reflect.TypeOf("")
	typeOfTime              = reflect.TypeOf(time.Time{})
	typeOfToggle            = reflect.TypeOf(Auto)
	typeOfMGS               = reflect.TypeOf((*MetaGetterSetter)(nil)).Elem()
	typeOfPropertyMap       = reflect.TypeOf((PropertyMap)(nil))
	typeOfError             = reflect.TypeOf((*error)(nil)).Elem()
)
