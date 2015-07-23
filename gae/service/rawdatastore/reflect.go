// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"reflect"
	"time"

	"github.com/luci/gae/service/blobstore"
)

var (
	typeOfBool              = reflect.TypeOf(false)
	typeOfBSKey             = reflect.TypeOf(blobstore.Key(""))
	typeOfByteSlice         = reflect.TypeOf([]byte(nil))
	typeOfByteString        = reflect.TypeOf(ByteString(nil))
	typeOfKey               = reflect.TypeOf((*Key)(nil)).Elem()
	typeOfPropertyConverter = reflect.TypeOf((*PropertyConverter)(nil)).Elem()
	typeOfFloat64           = reflect.TypeOf(float64(0))
	typeOfGeoPoint          = reflect.TypeOf(GeoPoint{})
	typeOfInt64             = reflect.TypeOf(int64(0))
	typeOfString            = reflect.TypeOf("")
	typeOfTime              = reflect.TypeOf(time.Time{})
	valueOfnilDSKey         = reflect.Zero(typeOfKey)
)
