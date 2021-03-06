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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/service/blobstore"
)

var (
	typeOfBSKey             = reflect.TypeOf(blobstore.Key(""))
	typeOfCursorCB          = reflect.TypeOf(CursorCB(nil))
	typeOfGeoPoint          = reflect.TypeOf(GeoPoint{})
	typeOfKey               = reflect.TypeOf((*Key)(nil))
	typeOfPropertyConverter = reflect.TypeOf((*PropertyConverter)(nil)).Elem()
	typeOfPropertyLoadSaver = reflect.TypeOf((*PropertyLoadSaver)(nil)).Elem()
	typeofProtoMessage      = reflect.TypeOf((*proto.Message)(nil)).Elem()
	typeOfMetaGetterSetter  = reflect.TypeOf((*MetaGetterSetter)(nil)).Elem()
	typeOfTime              = reflect.TypeOf(time.Time{})
	typeOfToggle            = reflect.TypeOf(Auto)
	typeOfPropertyMap       = reflect.TypeOf((PropertyMap)(nil))
	typeOfError             = reflect.TypeOf((*error)(nil)).Elem()
)
