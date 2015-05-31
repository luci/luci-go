// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type keyLeftRight struct{ key, left, right []byte }

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

func getFilledColl(s *memStore, fill []kv) *memCollection {
	if fill == nil {
		return nil
	}
	ret := s.MakePrivateCollection(nil)
	for _, i := range fill {
		ret.Set(i.k, i.v)
	}
	return ret
}

func TestCollision(t *testing.T) {
	t.Parallel()

	Convey("Test gkvCollide", t, func() {
		s := newMemStore()
		for _, tc := range testCollisionCases {
			Convey(tc.name, func() {
				left := getFilledColl(s, tc.left)
				right := getFilledColl(s, tc.right)
				i := 0
				gkvCollide(left, right, func(key, left, right []byte) {
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
