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
	ftt.Run(`ValidateSources`, t, func(t *ftt.Test) {
		sources := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "chromium.googlesource.com",
				Project:    "chromium/src",
				Ref:        "refs/heads/branch",
				CommitHash: "123456789012345678901234567890abcdefabcd",
				Position:   1,
			},
			Changelists: []*pb.GerritChange{
				{
					Host:     "chromium-review.googlesource.com",
					Project:  "infra/luci-go",
					Change:   12345,
					Patchset: 321,
				},
			},
			IsDirty: true,
		}
		t.Run(`Valid with sources`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSources(sources), should.BeNil)
		})
		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSources(nil), should.ErrLike(`unspecified`))
		})
		t.Run(`Gitiles commit`, func(t *ftt.Test) {
			t.Run(`Missing`, func(t *ftt.Test) {
				sources.GitilesCommit = nil
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`gitiles_commit: unspecified`))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				// protocol prefix should not be included.
				sources.GitilesCommit.Host = "https://service"
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`gitiles_commit: host: does not match`))
			})
		})
		t.Run(`Changelists`, func(t *ftt.Test) {
			t.Run(`Zero length`, func(t *ftt.Test) {
				sources.Changelists = nil
				assert.Loosely(t, ValidateSources(sources), should.BeNil)
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				sources.Changelists[0].Change = -1
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`changelists[0]: change: cannot be negative`))
			})
			t.Run(`Too many`, func(t *ftt.Test) {
				sources.Changelists = nil
				for i := range 11 {
					sources.Changelists = append(sources.Changelists,
						&pb.GerritChange{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci-go",
							Change:   int64(i + 1),
							Patchset: 321,
						},
					)
				}
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`changelists: exceeds maximum of 10 changelists`))
			})
			t.Run(`Duplicates`, func(t *ftt.Test) {
				sources.Changelists = nil
				for i := range 2 {
					sources.Changelists = append(sources.Changelists,
						&pb.GerritChange{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci-go",
							Change:   12345,
							Patchset: int64(i + 1),
						},
					)
				}
				assert.Loosely(t, ValidateSources(sources), should.ErrLike(`changelists[1]: duplicate change modulo patchset number; same change at changelists[0]`))
			})
		})
	})
	ftt.Run(`ValidateSourceSpec`, t, func(t *ftt.Test) {
		sourceSpec := &pb.SourceSpec{
			Sources: &pb.Sources{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/branch",
					CommitHash: "123456789012345678901234567890abcdefabcd",
					Position:   1,
				},
			},
			Inherit: false,
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			t.Run(`Sources only`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.BeNil)
			})
			t.Run(`Empty`, func(t *ftt.Test) {
				sourceSpec.Sources = nil
				assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.BeNil)
			})
			t.Run(`Inherit only`, func(t *ftt.Test) {
				sourceSpec.Sources = nil
				sourceSpec.Inherit = true
				assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.BeNil)
			})
			t.Run(`Nil`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateSourceSpec(nil), should.BeNil)
			})
		})
		t.Run(`Cannot specify inherit concurrently with sources`, func(t *ftt.Test) {
			assert.Loosely(t, sourceSpec.Sources, should.NotBeNil)
			sourceSpec.Inherit = true
			assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.ErrLike(`only one of inherit and sources may be set`))
		})
		t.Run(`Invalid Sources`, func(t *ftt.Test) {
			sourceSpec.Sources.GitilesCommit.Host = "b@d"
			assert.Loosely(t, ValidateSourceSpec(sourceSpec), should.ErrLike(`sources: gitiles_commit: host: does not match`))
		})
	})
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
