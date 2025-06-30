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
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestInvocationName(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseInvocationName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			id, err := ParseInvocationName("invocations/a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, id, should.Equal("a"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			_, err := ParseInvocationName("invocations/-")
			assert.Loosely(t, err, should.ErrLike(`does not match`))
		})

		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, InvocationName("a"), should.Equal("invocations/a"))
		})
	})

	ftt.Run("TryParseInvocationName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			id, ok := TryParseInvocationName("invocations/a")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, id, should.Equal("a"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			_, ok := TryParseInvocationName("invocations/-")
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Empty", func(t *ftt.Test) {
			_, ok := TryParseInvocationName("")
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}

func TestInvocationUtils(t *testing.T) {
	t.Parallel()
	ftt.Run(`Normalization normalizes tags`, t, func(t *ftt.Test) {
		inv := &pb.Invocation{
			Tags: StringPairs(
				"k2", "v21",
				"k2", "v20",
				"k3", "v30",
				"k1", "v1",
				"k3", "v31",
			),
		}

		NormalizeInvocation(inv)

		assert.Loosely(t, inv.Tags, should.Match(StringPairs(
			"k1", "v1",
			"k2", "v20",
			"k2", "v21",
			"k3", "v30",
			"k3", "v31",
		)))
	})
	ftt.Run(`Normalization normalizes gerrit changelists`, t, func(t *ftt.Test) {
		inv := &pb.Invocation{
			SourceSpec: &pb.SourceSpec{
				Sources: &pb.Sources{
					Changelists: []*pb.GerritChange{
						{
							Host:     "chromium-review.googlesource.com",
							Project:  "chromium/src",
							Change:   111,
							Patchset: 1,
						},
						{
							Host:     "a-review.googlesource.com",
							Project:  "chromium/src",
							Change:   444,
							Patchset: 1,
						},
						{
							Host:     "a-review.googlesource.com",
							Project:  "aaa",
							Change:   555,
							Patchset: 1,
						},
						{
							Host:     "chromium-review.googlesource.com",
							Project:  "chromium/src",
							Change:   333,
							Patchset: 1,
						},
						{
							Host:     "chromium-review.googlesource.com",
							Project:  "chromium/src",
							Change:   222,
							Patchset: 1,
						},
					},
				},
			},
		}

		NormalizeInvocation(inv)

		assert.Loosely(t, inv.SourceSpec.Sources.Changelists, should.Match([]*pb.GerritChange{
			{
				Host:     "a-review.googlesource.com",
				Project:  "aaa",
				Change:   555,
				Patchset: 1,
			},
			{
				Host:     "a-review.googlesource.com",
				Project:  "chromium/src",
				Change:   444,
				Patchset: 1,
			},
			{
				Host:     "chromium-review.googlesource.com",
				Project:  "chromium/src",
				Change:   111,
				Patchset: 1,
			},
			{
				Host:     "chromium-review.googlesource.com",
				Project:  "chromium/src",
				Change:   222,
				Patchset: 1,
			},
			{
				Host:     "chromium-review.googlesource.com",
				Project:  "chromium/src",
				Change:   333,
				Patchset: 1,
			},
		}))
	})
}

func TestValidateInvocation(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateInvocationExtendedPropertyKey`, t, func(t *ftt.Test) {
		t.Run(`Invalid key`, func(t *ftt.Test) {
			err := ValidateInvocationExtendedPropertyKey("mykey_")
			assert.Loosely(t, err, should.ErrLike(`does not match`))
		})
	})
	ftt.Run(`ValidateInvocationExtendedProperties`, t, func(t *ftt.Test) {
		t.Run(`Invalid key`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey_": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`key "mykey_": does not match`))
		})
		t.Run(`Max size of value`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue(strings.Repeat("a", MaxSizeInvocationExtendedPropertyValue)),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`exceeds the maximum size of `))
			assert.Loosely(t, err, should.ErrLike(`bytes`))
		})
		t.Run(`Missing @type`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`["mykey"]: must have a field "@type"`))
		})
		t.Run(`Invalid @type, missing slash`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`["mykey"]: "@type" value "some.package.MyMessage" must contain at least one "/" character`))
		})
		t.Run(`Invalid @type, invalid type url`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("[::1]/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`["mykey"]: "@type" value "[::1]/some.package.MyMessage":`))
		})
		t.Run(`Invalid @type, invalid type name`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/_some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`["mykey"]: "@type" type name "_some.package.MyMessage": does not match`))
		})
		t.Run(`Valid @type`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Max size of extended properties`, func(t *ftt.Test) {
			structValueLong := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
					"child_key_1": structpb.NewStringValue(strings.Repeat("a", MaxSizeInvocationExtendedPropertyValue-60)),
				},
			}
			extendedProperties := map[string]*structpb.Struct{
				"mykey_1": structValueLong,
				"mykey_2": structValueLong,
				"mykey_3": structValueLong,
				"mykey_4": structValueLong,
				"mykey_5": structValueLong,
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.ErrLike(`exceeds the maximum size of`))
			assert.Loosely(t, err, should.ErrLike(`bytes`))
		})
		t.Run(`Valid`, func(t *ftt.Test) {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
