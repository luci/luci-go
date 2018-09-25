// Copyright 2018 The LUCI Authors.
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

package mapper

import (
	"testing"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"

	. "github.com/smartystreets/goconvey/convey"
)

type paramsEntity struct {
	_id int `gae:"$id,1"`
	P   Params
}

func TestParams(t *testing.T) {
	t.Parallel()

	Convey("Serialization works", t, func() {
		ctx := gaetesting.TestingContext()

		roundTrip := func(p Params) {
			So(datastore.Put(ctx, &paramsEntity{P: p}), ShouldBeNil)
			e := paramsEntity{}
			So(datastore.Get(ctx, &e), ShouldBeNil)
			if len(p) == 0 {
				So(e.P, ShouldBeNil)
			} else {
				So(e.P, ShouldResemble, p)
			}
		}

		roundTrip(nil)
		roundTrip(Params{})
		roundTrip(Params{"a": "b"})
		roundTrip(Params{"a": map[string]interface{}{"b": "c"}})
	})
}
