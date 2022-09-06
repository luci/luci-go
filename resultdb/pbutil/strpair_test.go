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

package pbutil

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/strpair"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStringPairs(t *testing.T) {
	t.Parallel()
	Convey(`Works`, t, func() {
		So(StringPairs("k1", "v1", "k2", "v2"), ShouldResemble, []*pb.StringPair{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		})

		Convey(`and fails if provided with incomplete pairs`, func() {
			tokens := []string{"k1", "v1", "k2"}
			So(func() { StringPairs(tokens...) }, ShouldPanicWith,
				fmt.Sprintf("odd number of tokens in %q", tokens))
		})

		Convey(`when provided key:val string`, func() {
			pair, err := StringPairFromString("key/k:v")
			So(err, ShouldBeNil)
			So(pair, ShouldResembleProto, &pb.StringPair{Key: "key/k", Value: "v"})
		})
	})
	Convey(`StringPairFromStringUnvalidated`, t, func() {
		Convey(`valid key:val string`, func() {
			pair := StringPairFromStringUnvalidated("key/k:v")
			So(pair, ShouldResembleProto, &pb.StringPair{Key: "key/k", Value: "v"})
		})

		Convey(`invalid string with no colon`, func() {
			pair := StringPairFromStringUnvalidated("key/k")
			So(pair, ShouldResembleProto, &pb.StringPair{Key: "key/k", Value: ""})
		})

		Convey(`invalid chars in key`, func() {
			pair := StringPairFromStringUnvalidated("##key/k:v")
			So(pair, ShouldResembleProto, &pb.StringPair{Key: "##key/k", Value: "v"})
		})
	})
}

func TestValidateStringPair(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateStringPairs`, t, func() {
		Convey(`empty`, func() {
			err := ValidateStringPair(StringPair("", ""))
			So(err, ShouldErrLike, `key: unspecified`)
		})

		Convey(`invalid key`, func() {
			err := ValidateStringPair(StringPair("1", ""))
			So(err, ShouldErrLike, `key: does not match`)
		})

		Convey(`long key`, func() {
			err := ValidateStringPair(StringPair(strings.Repeat("a", 1000), ""))
			So(err, ShouldErrLike, `key length must be less or equal to 64`)
		})

		Convey(`long value`, func() {
			err := ValidateStringPair(StringPair("a", strings.Repeat("a", 1000)))
			So(err, ShouldErrLike, `value length must be less or equal to 256`)
		})

		Convey(`valid`, func() {
			err := ValidateStringPair(StringPair("a", "b"))
			So(err, ShouldBeNil)
		})
	})
}

func TestFromStrpairMap(t *testing.T) {
	t.Parallel()
	Convey(`FromStrpairMap`, t, func() {
		m := strpair.Map{}
		m.Add("k1", "v1")
		m.Add("k2", "v1")
		m.Add("k2", "v2")

		So(FromStrpairMap(m), ShouldResembleProto, StringPairs(
			"k1", "v1",
			"k2", "v1",
			"k2", "v2",
		))
	})
}
