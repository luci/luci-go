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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestBuildCEL(t *testing.T) {
	t.Parallel()

	ftt.Run("Bool", t, func(t *ftt.Test) {
		t.Run("fail", func(t *ftt.Test) {
			t.Run("empty predicates", func(t *ftt.Test) {
				_, err := NewBool([]string{})
				assert.Loosely(t, err, should.ErrLike("predicates are required"))
			})
			t.Run("wrong build field", func(t *ftt.Test) {
				_, err := NewBool([]string{`has(build.tags)`, `has(build.random)`})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run("work", func(t *ftt.Test) {
			predicates := []string{
				`(has(build.tags)||(has(build.input)))`,
				`has(build.input.properties.pro_key)`,
				`build.input.experiments.exists(e, e=="luci.buildbucket.exp")`,
				`string(build.output.properties.out_key) == "out_val"`,
			}

			t.Run("not match", func(t *ftt.Test) {
				pass, err := BoolEval(&pb.Build{}, predicates)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pass, should.BeFalse)
			})

			t.Run("partial match", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pass, should.BeFalse)
			})

			t.Run("pass", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pass, should.BeTrue)
			})
		})
	})

	ftt.Run("StringMap", t, func(t *ftt.Test) {
		t.Run("fail", func(t *ftt.Test) {
			t.Run("string unrecognized", func(t *ftt.Test) {
				_, err := NewStringMap(map[string]string{
					"b": `random_string_literal`,
				})
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("type unmatched", func(t *ftt.Test) {
				_, err := NewStringMap(map[string]string{
					"b": `build.tags`,
				})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
		t.Run("empty", func(t *ftt.Test) {
			smbc, err := NewStringMap(map[string]string{
				"a": `build.summary_markdown`,
				"b": `"random_string_literal"`,
				"c": `build.cancellation_markdown`,
			})
			assert.Loosely(t, err, should.BeNil)
			out, err := smbc.Eval(&pb.Build{})
			expected := map[string]string{
				"a": "",
				"b": "random_string_literal",
				"c": "",
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Resemble(expected))
		})
		t.Run("missing struct key", func(t *ftt.Test) {
			smbc, err := NewStringMap(map[string]string{
				"d": `string(build.input.properties.key)`,
			})
			assert.Loosely(t, err, should.BeNil)
			_, err = smbc.Eval(&pb.Build{})
			assert.Loosely(t, err, should.ErrLike("no such key: key"))
		})
		t.Run("work", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Resemble(expected))
		})
	})

	ftt.Run("tags", t, func(t *ftt.Test) {
		t.Run("tag exists", func(t *ftt.Test) {
			bbc, err := NewBool([]string{
				`build.tags.exists(t, t.key=="os")`,
				`build.tags.get_value("os")!=""`,
			})
			assert.Loosely(t, err, should.BeNil)
			t.Run("not matched", func(t *ftt.Test) {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "key",
							Value: "value",
						},
					},
				}
				pass, err := bbc.Eval(b)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pass, should.BeFalse)
			})
			t.Run("matched", func(t *ftt.Test) {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "Mac",
						},
					},
				}
				pass, err := bbc.Eval(b)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pass, should.BeTrue)
			})
		})
		t.Run("get_value", func(t *ftt.Test) {
			bbc, err := NewStringMap(map[string]string{"os": `build.tags.get_value("os")`})
			assert.Loosely(t, err, should.BeNil)
			t.Run("not found", func(t *ftt.Test) {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "key",
							Value: "value",
						},
					},
				}
				res, err := bbc.Eval(b)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(map[string]string{"os": ""}))
			})
			t.Run("found", func(t *ftt.Test) {
				b := &pb.Build{
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "Mac",
						},
					},
				}
				res, err := bbc.Eval(b)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(map[string]string{"os": "Mac"}))
			})
		})
	})

	ftt.Run("experments", t, func(t *ftt.Test) {
		bbc, err := NewStringMap(map[string]string{"experiments": `build.input.experiments.to_string()`})
		assert.Loosely(t, err, should.BeNil)
		t.Run("not found", func(t *ftt.Test) {
			b := &pb.Build{}
			res, err := bbc.Eval(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(map[string]string{"experiments": "None"}))
		})
		t.Run("found", func(t *ftt.Test) {
			b := &pb.Build{
				Input: &pb.Build_Input{
					Experiments: []string{
						"luci.buildbucket.exp2",
						"luci.buildbucket.exp1",
					},
				},
			}
			res, err := bbc.Eval(b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(map[string]string{"experiments": "luci.buildbucket.exp1|luci.buildbucket.exp2"}))
		})
	})

	ftt.Run("status", t, func(t *ftt.Test) {
		t.Run("status match", func(t *ftt.Test) {
			b := &pb.Build{
				Status: pb.Status_SUCCESS,
			}
			matched, err := BoolEval(b, []string{`build.status.to_string()=="SUCCESS"`})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.BeTrue)

			matched, err = BoolEval(b, []string{`build.status.to_string()=="INFRA_FAILURE"`})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, matched, should.BeFalse)
		})

		t.Run("get status", func(t *ftt.Test) {
			b := &pb.Build{
				Status: pb.Status_SUCCESS,
			}
			res, err := StringMapEval(b, map[string]string{"status": `build.status.to_string()`})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(map[string]string{"status": "SUCCESS"}))
		})
	})
}
