// Copyright 2020 The LUCI Authors.
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

package invocations

import (
	"reflect"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/span"

	. "github.com/smartystreets/goconvey/convey"
)

func InvocatoinIDSpanner(t *testing.T) {
	t.Parallel()
	Convey(`InvocationID`, t, func() {
		var b span.Buffer
		test := func(goValue, spValue interface{}) {
			// ToSpanner
			actualSPValue := span.ToSpanner(goValue)
			So(actualSPValue, ShouldResemble, spValue)

			// FromSpanner
			row, err := spanner.NewRow([]string{"a"}, []interface{}{actualSPValue})
			So(err, ShouldBeNil)
			goPtr := reflect.New(reflect.TypeOf(goValue))
			err = b.FromSpanner(row, goPtr.Interface())
			So(err, ShouldBeNil)
			So(goPtr.Elem().Interface(), ShouldResemble, goValue)
		}

		Convey(`*InvocationID`, t, func() {
			test(ID("a"), "ca978112:a")
		})

		Convey(`*InvocationIDSet`, t, func() {
			test(NewIDSet("a", "b"), []string{"3e23e816:b", "ca978112:a"})
		})
	})
}
