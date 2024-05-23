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

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInvocationName(t *testing.T) {
	t.Parallel()
	Convey("ParseInvocationName", t, func() {
		Convey("Parse", func() {
			id, err := ParseInvocationName("invocations/a")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, "a")
		})

		Convey("Invalid", func() {
			_, err := ParseInvocationName("invocations/-")
			So(err, ShouldErrLike, `does not match`)
		})

		Convey("Format", func() {
			So(InvocationName("a"), ShouldEqual, "invocations/a")
		})

	})
}

func TestInvocationUtils(t *testing.T) {
	t.Parallel()
	Convey(`Normalization normalizes tags`, t, func() {
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

		So(inv.Tags, ShouldResembleProto, StringPairs(
			"k1", "v1",
			"k2", "v20",
			"k2", "v21",
			"k3", "v30",
			"k3", "v31",
		))
	})
	Convey(`Normalization normalizes gerrit changelists`, t, func() {
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

		So(inv.SourceSpec.Sources.Changelists, ShouldResembleProto, []*pb.GerritChange{
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
		})
	})
}

func TestValidateInvocation(t *testing.T) {
	t.Parallel()
	Convey(`ValidateSources`, t, func() {
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
		Convey(`Valid with sources`, func() {
			So(ValidateSources(sources), ShouldBeNil)
		})
		Convey(`Nil`, func() {
			So(ValidateSources(nil), ShouldErrLike, `unspecified`)
		})
		Convey(`Gitiles commit`, func() {
			Convey(`Missing`, func() {
				sources.GitilesCommit = nil
				So(ValidateSources(sources), ShouldErrLike, `gitiles_commit: unspecified`)
			})
			Convey(`Invalid`, func() {
				// protocol prefix should not be included.
				sources.GitilesCommit.Host = "https://service"
				So(ValidateSources(sources), ShouldErrLike, `gitiles_commit: host: does not match`)
			})
		})
		Convey(`Changelists`, func() {
			Convey(`Zero length`, func() {
				sources.Changelists = nil
				So(ValidateSources(sources), ShouldBeNil)
			})
			Convey(`Invalid`, func() {
				sources.Changelists[0].Change = -1
				So(ValidateSources(sources), ShouldErrLike, `changelists[0]: change: cannot be negative`)
			})
			Convey(`Too many`, func() {
				sources.Changelists = nil
				for i := 0; i < 11; i++ {
					sources.Changelists = append(sources.Changelists,
						&pb.GerritChange{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci-go",
							Change:   int64(i + 1),
							Patchset: 321,
						},
					)
				}
				So(ValidateSources(sources), ShouldErrLike, `changelists: exceeds maximum of 10 changelists`)
			})
			Convey(`Duplicates`, func() {
				sources.Changelists = nil
				for i := 0; i < 2; i++ {
					sources.Changelists = append(sources.Changelists,
						&pb.GerritChange{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci-go",
							Change:   12345,
							Patchset: int64(i + 1),
						},
					)
				}
				So(ValidateSources(sources), ShouldErrLike, `changelists[1]: duplicate change modulo patchset number; same change at changelists[0]`)
			})
		})
	})
	Convey(`ValidateSourceSpec`, t, func() {
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
		Convey(`Valid`, func() {
			Convey(`Sources only`, func() {
				So(ValidateSourceSpec(sourceSpec), ShouldBeNil)
			})
			Convey(`Empty`, func() {
				sourceSpec.Sources = nil
				So(ValidateSourceSpec(sourceSpec), ShouldBeNil)
			})
			Convey(`Inherit only`, func() {
				sourceSpec.Sources = nil
				sourceSpec.Inherit = true
				So(ValidateSourceSpec(sourceSpec), ShouldBeNil)
			})
			Convey(`Nil`, func() {
				So(ValidateSourceSpec(nil), ShouldBeNil)
			})
		})
		Convey(`Cannot specify inherit concurrently with sources`, func() {
			So(sourceSpec.Sources, ShouldNotBeNil)
			sourceSpec.Inherit = true
			So(ValidateSourceSpec(sourceSpec), ShouldErrLike, `only one of inherit and sources may be set`)
		})
		Convey(`Invalid Sources`, func() {
			sourceSpec.Sources.GitilesCommit.Host = "b@d"
			So(ValidateSourceSpec(sourceSpec), ShouldErrLike, `sources: gitiles_commit: host: does not match`)
		})
	})
	Convey(`ValidateInvocationExtendedPropertyKey`, t, func() {
		Convey(`Invalid key`, func() {
			err := ValidateInvocationExtendedPropertyKey("mykey_")
			So(err, ShouldErrLike, `does not match`)
		})
	})
	Convey(`ValidateInvocationExtendedProperties`, t, func() {
		Convey(`Invalid key`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey_": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldErrLike, `key "mykey_": does not match`)
		})
		Convey(`Max size of value`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue(strings.Repeat("a", MaxSizeInvocationExtendedPropertyValue)),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldErrLike, `["mykey"]: exceeds the maximum size of `, `bytes`)
		})
		Convey(`Missing @type`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldErrLike, `["mykey"]: must have a field "@type"`)
		})
		Convey(`Invalid @type, missing slash`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldErrLike, `["mykey"]: "@type" value "some.package.MyMessage" must contain at least one "/" character`)
		})
		Convey(`Invalid @type, invalid type url`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("[::1]/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldErrLike, `["mykey"]: "@type" value "[::1]/some.package.MyMessage":`)
		})
		Convey(`Invalid @type, invalid type name`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/_some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldErrLike, `["mykey"]: "@type" type name "_some.package.MyMessage": does not match`)
		})
		Convey(`Valid @type`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldBeNil)
		})
		Convey(`Max size of extended properties`, func() {
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
			So(err, ShouldErrLike, `exceeds the maximum size of`, `bytes`)
		})
		Convey(`Valid`, func() {
			extendedProperties := map[string]*structpb.Struct{
				"mykey": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			err := ValidateInvocationExtendedProperties(extendedProperties)
			So(err, ShouldBeNil)
		})
	})
}
