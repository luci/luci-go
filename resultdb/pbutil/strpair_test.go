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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestStringPairs(t *testing.T) {
	t.Parallel()
	ftt.Run(`Works`, t, func(t *ftt.Test) {
		assert.Loosely(t, StringPairs("k1", "v1", "k2", "v2"), should.Match([]*pb.StringPair{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		}))

		t.Run(`and fails if provided with incomplete pairs`, func(t *ftt.Test) {
			tokens := []string{"k1", "v1", "k2"}
			assert.Loosely(t, func() { StringPairs(tokens...) }, should.PanicLikeString(
				fmt.Sprintf("odd number of tokens in %q", tokens)))
		})

		t.Run(`when provided key:val string`, func(t *ftt.Test) {
			pair, err := StringPairFromString("key/k:v")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pair, should.Match(&pb.StringPair{Key: "key/k", Value: "v"}))
		})

		t.Run(`when provided multiline value string`, func(t *ftt.Test) {
			pair, err := StringPairFromString("key/k:multiline\nstring\nvalue")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pair, should.Match(&pb.StringPair{Key: "key/k", Value: "multiline\nstring\nvalue"}))
		})
	})
	ftt.Run(`StringPairFromStringUnvalidated`, t, func(t *ftt.Test) {
		t.Run(`valid key:val string`, func(t *ftt.Test) {
			pair := StringPairFromStringUnvalidated("key/k:v")
			assert.Loosely(t, pair, should.Match(&pb.StringPair{Key: "key/k", Value: "v"}))
		})

		t.Run(`invalid string with no colon`, func(t *ftt.Test) {
			pair := StringPairFromStringUnvalidated("key/k")
			assert.Loosely(t, pair, should.Match(&pb.StringPair{Key: "key/k", Value: ""}))
		})

		t.Run(`invalid chars in key`, func(t *ftt.Test) {
			pair := StringPairFromStringUnvalidated("##key/k:v")
			assert.Loosely(t, pair, should.Match(&pb.StringPair{Key: "##key/k", Value: "v"}))
		})
	})
}

func TestValidateStringPair(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateStringPair`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("", ""), false)
			assert.Loosely(t, err, should.ErrLike(`key: unspecified`))
		})

		t.Run(`invalid key`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("1", ""), false)
			assert.Loosely(t, err, should.ErrLike(`key: does not match`))
		})

		t.Run(`long key`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair(strings.Repeat("a", 1000), ""), false)
			assert.Loosely(t, err, should.ErrLike(`key: length must be less or equal to 64 bytes`))
		})

		t.Run(`long value`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("a", strings.Repeat("a", 1000)), false)
			assert.Loosely(t, err, should.ErrLike(`value: length must be less or equal to 256 bytes`))
		})

		t.Run(`multiline value`, func(t *ftt.Test) {
			t.Run(`strict mode`, func(t *ftt.Test) {
				err := ValidateStringPair(StringPair("a", "multi\nline\nvalue"), true)
				assert.Loosely(t, err, should.ErrLike(`value: non-printable rune '\n' at byte index 5`))
			})
			t.Run(`non-strict mode`, func(t *ftt.Test) {
				err := ValidateStringPair(StringPair("a", "multi\nline\nvalue"), false)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run(`valid`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("a", "b"), false)
			assert.Loosely(t, err, should.BeNil)
		})
	})
	ftt.Run(`ValidateRootInvocationTags`, t, func(t *ftt.Test) {
		t.Run(`valid`, func(t *ftt.Test) {
			tags := []*pb.StringPair{
				StringPair("a", "b"),
				StringPair("c", "d"),
			}
			err := ValidateRootInvocationTags(tags)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`too large`, func(t *ftt.Test) {
			// Create a tag that is just under the limit for a single tag.
			// The proto.Size(p) calculation includes overhead for the StringPair message itself,
			// plus the key and value strings.
			// A key of 64 bytes and a value of 256 bytes is 320 bytes of data.
			// To this we add the overhead for this StringPair message (5 bytes),
			// giving a total size of 325 bytes.
			// maxRootInvocationTagsSize = 16 * 1024 = 16384 bytes.
			// So, we can fit about 16384 / 325 = 50.4 tags.
			// Let's create 51 tags to exceed the limit.
			tags := make([]*pb.StringPair, 51)
			for i := 0; i < 51; i++ {
				tags[i] = StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
			}
			err := ValidateRootInvocationTags(tags)
			assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("got 16575 bytes; exceeds the maximum size of %d bytes", maxRootInvocationTagsSize)))
		})
	})
	ftt.Run(`ValidateWorkUnitTags`, t, func(t *ftt.Test) {
		t.Run(`too large`, func(t *ftt.Test) {
			// 64 (key) + 256 (value) + 5 (overhead) = 325 bytes per tag.
			// maxRootInvocationTagsSize = 16 * 1024 = 16384 bytes.
			// So, we can fit about 16384 / 325 = 50.4 tags.
			// Let's create 51 tags to exceed the limit.
			tags := make([]*pb.StringPair, 51)
			for i := 0; i < 51; i++ {
				tags[i] = StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
			}
			err := ValidateRootInvocationTags(tags)
			assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("got 16575 bytes; exceeds the maximum size of %d bytes", maxWorkUnitTagsSize)))
		})
	})
}

func TestFromStrpairMap(t *testing.T) {
	t.Parallel()
	ftt.Run(`FromStrpairMap`, t, func(t *ftt.Test) {
		m := strpair.Map{}
		m.Add("k1", "v1")
		m.Add("k2", "v1")
		m.Add("k2", "v2")

		assert.Loosely(t, FromStrpairMap(m), should.Match(StringPairs(
			"k1", "v1",
			"k2", "v1",
			"k2", "v2",
		)))
	})
}

func TestStringPairsEqual(t *testing.T) {
	t.Parallel()
	ftt.Run("StringPairsEqual", t, func(t *ftt.Test) {
		p1 := StringPairs("k1", "v1", "k2", "v2")
		p2 := StringPairs("k1", "v1", "k2", "v2")
		p3 := StringPairs("k2", "v2", "k1", "v1") // different order
		p4 := StringPairs("k1", "v1")             // different length
		p5 := StringPairs("k1", "v1", "k2", "v3") // different value

		t.Run("equal", func(t *ftt.Test) {
			assert.Loosely(t, StringPairsEqual(p1, p2), should.BeTrue)
		})
		t.Run("different order", func(t *ftt.Test) {
			assert.Loosely(t, StringPairsEqual(p1, p3), should.BeFalse)
		})
		t.Run("different length", func(t *ftt.Test) {
			assert.Loosely(t, StringPairsEqual(p1, p4), should.BeFalse)
		})
		t.Run("different value", func(t *ftt.Test) {
			assert.Loosely(t, StringPairsEqual(p1, p5), should.BeFalse)
		})
		t.Run("one nil", func(t *ftt.Test) {
			assert.Loosely(t, StringPairsEqual(p1, nil), should.BeFalse)
			assert.Loosely(t, StringPairsEqual(nil, p1), should.BeFalse)
		})
		t.Run("both nil", func(t *ftt.Test) {
			assert.Loosely(t, StringPairsEqual(nil, nil), should.BeTrue)
		})
	})
}
