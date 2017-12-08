// Copyright 2017 The LUCI Authors.
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

package utils

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateTags(t *testing.T) {
	Convey("ValidateTags ok", t, func() {
		So(ValidateTags(nil), ShouldBeNil)
		So(ValidateTags([]string{"k1:v1", "k2:v2"}), ShouldBeNil)
		So(ValidateTags([]string{"k1:v1:more:stuff"}), ShouldBeNil)
	})

	Convey("ValidateTags errors", t, func() {
		var many []string
		for i := 0; i < maxTagCount+1; i++ {
			many = append(many, "k:v")
		}
		So(ValidateTags(many), ShouldErrLike, "too many tags given")

		So(
			ValidateTags([]string{"k:v", "not-kv"}),
			ShouldErrLike,
			"tag #2: not in <key>:<value> form")
		So(
			ValidateTags([]string{strings.Repeat("k", maxTagKeySize+1) + ":v"}),
			ShouldErrLike,
			"tag #1: the key length must not exceed 128")
		So(
			ValidateTags([]string{"k:" + strings.Repeat("v", maxTagValueSize+1)}),
			ShouldErrLike,
			"tag #1: the value length must not exceed 1024")
	})
}
