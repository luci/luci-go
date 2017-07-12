// Copyright 2016 The LUCI Authors.
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

package shards

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShards(t *testing.T) {
	Convey("Build shards, serialize and deserialize", t, func() {
		// Build sharded set of 500 strings.
		set := make(Set, 10)
		for i := 0; i < 500; i++ {
			set.Insert([]byte(fmt.Sprintf("blob #%d", i)))
		}

		// Readd same strings. Should be noop, they are already in the set.
		for i := 0; i < 500; i++ {
			set.Insert([]byte(fmt.Sprintf("blob #%d", i)))
		}

		// Make sure shards are balanced, and all data is there.
		totalLen := 0
		for _, shard := range set {
			So(len(shard), ShouldBeGreaterThan, 45)
			So(len(shard), ShouldBeLessThan, 55)
			totalLen += len(shard)
		}
		So(totalLen, ShouldEqual, 500)

		// Serialize and deserialize first shard, should be left unchanged.
		shard, err := ParseShard(set[0].Serialize())
		So(err, ShouldBeNil)
		So(shard, ShouldResemble, set[0])
	})

	Convey("Empty shards serialization", t, func() {
		var shard Shard
		deserialized, err := ParseShard(shard.Serialize())
		So(err, ShouldBeNil)
		So(len(deserialized), ShouldEqual, 0)
	})

	Convey("Deserialize garbage", t, func() {
		_, err := ParseShard([]byte("blah"))
		So(err, ShouldNotBeNil)
	})
}
