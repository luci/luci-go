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
		assert.Loosely(t, StringPairs("k1", "v1", "k2", "v2"), should.Resemble([]*pb.StringPair{
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
			assert.Loosely(t, pair, should.Resemble(&pb.StringPair{Key: "key/k", Value: "v"}))
		})

		t.Run(`when provided multiline value string`, func(t *ftt.Test) {
			pair, err := StringPairFromString("key/k:multiline\nstring\nvalue")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pair, should.Resemble(&pb.StringPair{Key: "key/k", Value: "multiline\nstring\nvalue"}))
		})
	})
	ftt.Run(`StringPairFromStringUnvalidated`, t, func(t *ftt.Test) {
		t.Run(`valid key:val string`, func(t *ftt.Test) {
			pair := StringPairFromStringUnvalidated("key/k:v")
			assert.Loosely(t, pair, should.Resemble(&pb.StringPair{Key: "key/k", Value: "v"}))
		})

		t.Run(`invalid string with no colon`, func(t *ftt.Test) {
			pair := StringPairFromStringUnvalidated("key/k")
			assert.Loosely(t, pair, should.Resemble(&pb.StringPair{Key: "key/k", Value: ""}))
		})

		t.Run(`invalid chars in key`, func(t *ftt.Test) {
			pair := StringPairFromStringUnvalidated("##key/k:v")
			assert.Loosely(t, pair, should.Resemble(&pb.StringPair{Key: "##key/k", Value: "v"}))
		})
	})
}

func TestValidateStringPair(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateStringPairs`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("", ""))
			assert.Loosely(t, err, should.ErrLike(`key: unspecified`))
		})

		t.Run(`invalid key`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("1", ""))
			assert.Loosely(t, err, should.ErrLike(`key: does not match`))
		})

		t.Run(`long key`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair(strings.Repeat("a", 1000), ""))
			assert.Loosely(t, err, should.ErrLike(`key length must be less or equal to 64`))
		})

		t.Run(`long value`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("a", strings.Repeat("a", 1000)))
			assert.Loosely(t, err, should.ErrLike(`value length must be less or equal to 256`))
		})

		t.Run(`multiline value`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("a", "multi\nline\nvalue"))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`valid`, func(t *ftt.Test) {
			err := ValidateStringPair(StringPair("a", "b"))
			assert.Loosely(t, err, should.BeNil)
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

		assert.Loosely(t, FromStrpairMap(m), should.Resemble(StringPairs(
			"k1", "v1",
			"k2", "v1",
			"k2", "v2",
		)))
	})
}
