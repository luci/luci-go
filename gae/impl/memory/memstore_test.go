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

package memory

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type keyLeftRight struct{ key, left, right []byte }

type kv struct{ k, v []byte }

var testCollisionCases = []struct {
	name        string
	left, right []kv // inserts into left and right collections
	expect      []keyLeftRight
}{
	{
		name: "nil",
	},
	{
		name:  "empty",
		left:  []kv{},
		right: []kv{},
	},
	{
		name: "all old",
		left: []kv{
			{cat(1), cat()},
			{cat(0), cat()},
		},
		expect: []keyLeftRight{
			{cat(0), cat(), nil},
			{cat(1), cat(), nil},
		},
	},
	{
		name: "all new",
		right: []kv{
			{cat(1), cat()},
			{cat(0), cat()},
		},
		expect: []keyLeftRight{
			{cat(0), nil, cat()},
			{cat(1), nil, cat()},
		},
	},
	{
		name: "new vals",
		left: []kv{
			{cat(1), cat("hi")},
			{cat(0), cat("newb")},
		},
		right: []kv{
			{cat(0), cat(2.5)},
			{cat(1), cat(58)},
		},
		expect: []keyLeftRight{
			{cat(0), cat("newb"), cat(2.5)},
			{cat(1), cat("hi"), cat(58)},
		},
	},
	{
		name: "mixed",
		left: []kv{
			{cat(1), cat("one")},
			{cat(0), cat("hi")},
			{cat(6), cat()},
			{cat(3), cat(1.3)},
			{cat(2), []byte("zoop")},
			{cat(-1), cat("bob")},
		},
		right: []kv{
			{cat(3), cat(1)},
			{cat(1), cat(58)},
			{cat(0), cat(2.5)},
			{cat(4), cat(1337)},
			{cat(2), cat("ski", 7)},
			{cat(20), cat("nerd")},
		},
		expect: []keyLeftRight{
			{cat(-1), cat("bob"), nil},
			{cat(0), cat("hi"), cat(2.5)},
			{cat(1), cat("one"), cat(58)},
			{cat(2), []byte("zoop"), cat("ski", 7)},
			{cat(3), cat(1.3), cat(1)},
			{cat(4), nil, cat(1337)},
			{cat(6), cat(), nil},
			{cat(20), nil, cat("nerd")},
		},
	},
}

func getFilledColl(fill []kv) memCollection {
	if fill == nil {
		return nil
	}
	store := newMemStore()
	ret := store.GetOrCreateCollection("")
	for _, i := range fill {
		ret.Set(i.k, i.v)
	}
	return store.Snapshot().GetCollection("")
}

func TestCollision(t *testing.T) {
	t.Parallel()

	Convey("Test memStoreCollide", t, func() {
		for _, tc := range testCollisionCases {
			Convey(tc.name, func() {
				left := getFilledColl(tc.left)
				right := getFilledColl(tc.right)
				i := 0
				memStoreCollide(left, right, func(key, left, right []byte) {
					e := tc.expect[i]
					So(key, ShouldResemble, e.key)
					So(left, ShouldResemble, e.left)
					So(right, ShouldResemble, e.right)
					i++
				})
				So(i, ShouldEqual, len(tc.expect))
			})
		}
	})
}
