// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assertions

import (
	"fmt"

	"github.com/luci/luci-go/common/render"
	"github.com/smartystreets/goconvey/convey"
)

// ShouldResembleV is a single-argument goconvey assertion that performs a
// deep analysis of a type, returning an error string if it differs from the
// expected type.
//
// This is goconvey's ShouldResemble with error strings that include differences
// in pointer values.
func ShouldResembleV(actual interface{}, expected ...interface{}) string {
	if matches := convey.ShouldResemble(actual, expected...); matches != "" {
		return fmt.Sprintf("Expected: '%s'\nActual:   '%s'\n(Should resemble)!",
			render.StringDeep(actual), render.StringDeep(expected[0]))
	}
	return ""
}

// ShouldNotResembleV is a single-argument goconvey assertion that performs a
// deep analysis of a type, returning an error string if it doesn't differ from
// the expected type.
//
// This is goconvey's ShouldNotResemble with error strings that include
// differences in pointer values.
func ShouldNotResembleV(actual interface{}, expected ...interface{}) string {
	if matches := convey.ShouldNotResemble(actual, expected...); matches != "" {
		return fmt.Sprintf("Expected        '%s'\nto NOT resemble '%s'\n(but it did)!",
			render.StringDeep(actual), render.StringDeep(expected[0]))
	}
	return ""
}
