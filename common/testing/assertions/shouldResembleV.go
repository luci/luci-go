// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assertions

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/luci/luci-go/common/render"
)

type serialized struct {
	Message  string
	Expected string
	Actual   string
}

// ShouldResembleV is a single-argument goconvey assertion that performs a
// deep analysis of a type, returning an error string if it differs from the
// expected type.
//
// This is goconvey's ShouldResemble with error strings that include differences
// in pointer values.
func ShouldResembleV(actual interface{}, expected ...interface{}) string {
	if reflect.DeepEqual(actual, expected[0]) {
		return ""
	}

	act := render.DeepRender(actual)
	exp := render.DeepRender(expected[0])

	d, _ := json.Marshal(&serialized{
		Message: fmt.Sprintf(""+
			"Expected: '%s'\n"+
			"Actual:   '%s'\n"+
			"(Should resemble)!", exp, act),
		Expected: exp,
		Actual:   act,
	})
	return string(d)
}

// ShouldNotResembleV is a single-argument goconvey assertion that performs a
// deep analysis of a type, returning an error string if it doesn't differ from
// the expected type.
//
// This is goconvey's ShouldNotResemble with error strings that include
// differences in pointer values.
func ShouldNotResembleV(actual interface{}, expected ...interface{}) string {
	if !reflect.DeepEqual(actual, expected[0]) {
		return ""
	}
	return fmt.Sprintf(""+
		"Expected        '%s'\n"+
		"to NOT resemble '%s'\n"+
		"(but it did)!", render.DeepRender(expected[0]), render.DeepRender(actual))
}
