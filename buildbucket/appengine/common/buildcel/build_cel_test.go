// Copyright 2024 The LUCI Authors.
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

package buildcel

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildCEL(t *testing.T) {
	t.Parallel()

	Convey("Bool", t, func() {
		Convey("fail", func() {
			Convey("empty predicates", func() {
				_, err := NewBool([]string{})
				So(err, ShouldErrLike, "predicates are required")
			})
			Convey("wrong build field", func() {
				_, err := NewBool([]string{`has(build.tags)`, `has(build.random)`})
				So(err, ShouldNotBeNil)
			})
		})

		Convey("work", func() {
			predicates := []string{
				`(has(build.tags)||(has(build.input)))`,
				`has(build.input.properties.pro_key)`,
				`build.input.experiments.exists(e, e=="luci.buildbucket.exp")`,
				`string(build.output.properties.out_key) == "out_val"`,
			}

			Convey("not match", func() {
				pass, err := BoolEval(&pb.Build{}, predicates)
				So(err, ShouldBeNil)
				So(pass, ShouldBeFalse)
			})

			Convey("partial match", func() {
				b := &pb.Build{
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					},
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "mac",
						},
					},
				}
				pass, err := BoolEval(b, predicates)
				So(err, ShouldBeNil)
				So(pass, ShouldBeFalse)
			})

			Convey("pass", func() {
				b := &pb.Build{
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"pro_key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
						Experiments: []string{
							"luci.buildbucket.exp",
						},
					},
					Output: &pb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"out_key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "out_val",
									},
								},
							},
						},
					},
				}
				pass, err := BoolEval(b, predicates)
				So(err, ShouldBeNil)
				So(pass, ShouldBeTrue)
			})
		})
	})

	Convey("StringMap", t, func() {
		Convey("fail", func() {
			Convey("string unrecognized", func() {
				_, err := NewStringMap(map[string]string{
					"b": `random_string_literal`,
				})
				So(err, ShouldNotBeNil)
			})
			Convey("type unmatched", func() {
				_, err := NewStringMap(map[string]string{
					"b": `build.tags`,
				})
				So(err, ShouldNotBeNil)
			})
		})
		Convey("empty", func() {
			smbc, err := NewStringMap(map[string]string{
				"a": `build.summary_markdown`,
				"b": `"random_string_literal"`,
				"c": `build.cancellation_markdown`,
			})
			So(err, ShouldBeNil)
			out, err := smbc.Eval(&pb.Build{})
			expected := map[string]string{
				"a": "",
				"b": "random_string_literal",
				"c": "",
			}
			So(err, ShouldBeNil)
			So(out, ShouldResemble, expected)
		})
		Convey("missing struct key", func() {
			smbc, err := NewStringMap(map[string]string{
				"d": `string(build.input.properties.key)`,
			})
			So(err, ShouldBeNil)
			_, err = smbc.Eval(&pb.Build{})
			So(err, ShouldErrLike, "no such key: key")
		})
		Convey("work", func() {
			fields := map[string]string{
				"a": `build.summary_markdown`,
				"b": `"random_string_literal"`,
				"c": `build.cancellation_markdown`,
				"d": `string(build.input.properties.key)`,
			}
			b := &pb.Build{
				SummaryMarkdown:      "summary",
				CancellationMarkdown: "cancel",
				Tags: []*pb.StringPair{
					{
						Key:   "os",
						Value: "mac",
					},
				},
				Input: &pb.Build_Input{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					},
				},
			}
			expected := map[string]string{
				"a": "summary",
				"b": "random_string_literal",
				"c": "cancel",
				"d": "value",
			}
			out, err := StringMapEval(b, fields)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, expected)
		})
	})

	Convey("tags", t, func() {
		Convey("tag exists", func() {
			bbc, err := NewBool([]string{
				`build.tags.exists(t, t.key=="os")`,
				`build.tags.get_value("os")!=""`,
			})
			So(err, ShouldBeNil)
			Convey("not matched", func() {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "key",
							Value: "value",
						},
					},
				}
				pass, err := bbc.Eval(b)
				So(err, ShouldBeNil)
				So(pass, ShouldBeFalse)
			})
			Convey("matched", func() {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "Mac",
						},
					},
				}
				pass, err := bbc.Eval(b)
				So(err, ShouldBeNil)
				So(pass, ShouldBeTrue)
			})
		})
		Convey("get_value", func() {
			bbc, err := NewStringMap(map[string]string{"os": `build.tags.get_value("os")`})
			So(err, ShouldBeNil)
			Convey("not found", func() {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "key",
							Value: "value",
						},
					},
				}
				res, err := bbc.Eval(b)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, map[string]string{"os": ""})
			})
			Convey("found", func() {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "Mac",
						},
					},
				}
				res, err := bbc.Eval(b)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, map[string]string{"os": "Mac"})
			})
		})
	})

	Convey("experments", t, func() {
		bbc, err := NewStringMap(map[string]string{"experiments": `build.input.experiments.to_string()`})
		So(err, ShouldBeNil)
		Convey("not found", func() {
			b := &pb.Build{}
			res, err := bbc.Eval(b)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, map[string]string{"experiments": "None"})
		})
		Convey("found", func() {
			b := &pb.Build{
				Input: &pb.Build_Input{
					Experiments: []string{
						"luci.buildbucket.exp2",
						"luci.buildbucket.exp1",
					},
				},
			}
			res, err := bbc.Eval(b)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, map[string]string{"experiments": "luci.buildbucket.exp1|luci.buildbucket.exp2"})
		})
	})

	Convey("status", t, func() {
		Convey("status match", func() {
			b := &pb.Build{
				Status: pb.Status_SUCCESS,
			}
			matched, err := BoolEval(b, []string{`build.status.to_string()=="SUCCESS"`})
			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)

			matched, err = BoolEval(b, []string{`build.status.to_string()=="INFRA_FAILURE"`})
			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
		})

		Convey("get status", func() {
			b := &pb.Build{
				Status: pb.Status_SUCCESS,
			}
			res, err := StringMapEval(b, map[string]string{"status": `build.status.to_string()`})
			So(err, ShouldBeNil)
			So(res, ShouldResemble, map[string]string{"status": "SUCCESS"})
		})
	})
}
