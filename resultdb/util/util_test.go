// Copyright 2019 The LUCI Authors.
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

package util

import (
	"fmt"
	"testing"

	resultspb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStringPairs(t *testing.T) {
	Convey(`Works`, t, func() {
		So(StringPairs("k1", "v1", "k2", "v2"), ShouldResemble, []*resultspb.StringPair{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		})

		Convey(`and fails if provided with incomplete pairs`, func() {
			tokens := []string{"k1", "v1", "k2"}
			So(func() { StringPairs(tokens...) }, ShouldPanicWith,
				fmt.Sprintf("odd number of tokens in %q", tokens))
		})
	})
}
