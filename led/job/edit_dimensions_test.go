// Copyright 2020 The LUCI Authors.
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

package job

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestMakeDimensionEditCommands(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		name   string
		cmds   []string
		err    string
		expect DimensionEditCommands
	}{
		{name: "empty"},
		{
			name: "err no op",
			cmds: []string{"bad"},
			err:  "op was missing",
		},
		{
			name: "err no value",
			cmds: []string{"bad+="},
			err:  "empty value not allowed for operator",
		},
		{
			name: "err bad expiration",
			cmds: []string{"bad=value@dogs"},
			err:  `parsing expiration "dogs"`,
		},
		{
			name: "err unwanted expiration",
			cmds: []string{"bad-=value@123"},
			err:  "expiration seconds not allowed",
		},
		{
			name: "reset",
			cmds: []string{
				"key=",
			},
			expect: DimensionEditCommands{
				"key": &DimensionEditCommand{
					SetValues: []ExpiringValue{},
				},
			},
		},
		{
			name: "set",
			cmds: []string{
				"key=value",
				"key=other_value@123",
				"something=value", // ignored on 'apply', but will show up here
				"something=value@123",
			},
			expect: DimensionEditCommands{
				"key": &DimensionEditCommand{
					SetValues: []ExpiringValue{
						{Value: "value"},
						{Value: "other_value", Expiration: 123 * time.Second},
					},
				},
				"something": &DimensionEditCommand{
					SetValues: []ExpiringValue{
						{Value: "value"},
						{Value: "value", Expiration: 123 * time.Second},
					},
				},
			},
		},
		{
			name: "add",
			cmds: []string{
				"key+=value",
				"key+=other_value@123",
				"something+=value", // ignored on 'apply', but will show up here
				"something+=value@123",
			},
			expect: DimensionEditCommands{
				"key": &DimensionEditCommand{
					AddValues: []ExpiringValue{
						{Value: "value"},
						{Value: "other_value", Expiration: 123 * time.Second},
					},
				},
				"something": &DimensionEditCommand{
					AddValues: []ExpiringValue{
						{Value: "value"},
						{Value: "value", Expiration: 123 * time.Second},
					},
				},
			},
		},
		{
			name: "remove",
			cmds: []string{
				"key-=value",
				"key-=other_value",
			},
			expect: DimensionEditCommands{
				"key": &DimensionEditCommand{
					RemoveValues: []string{"value", "other_value"},
				},
			},
		},
	}

	ftt.Run(`MakeDimensionEditCommands`, t, func(t *ftt.Test) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				dec, err := MakeDimensionEditCommands(tc.cmds)
				if tc.err == "" {
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dec, should.Match(tc.expect))
				} else {
					assert.Loosely(t, err, should.ErrLike(tc.err))
				}
			})
		}
	})
}

func TestSetDimensions(t *testing.T) {
	t.Parallel()

	runCases(t, "SetDimensions", []testCase{
		{
			name: "nil",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.SetDimensions(nil)
				})
				assert.Loosely(t, mustGetDimensions(t, jd), should.BeEmpty)
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				baselineDims(t, jd)

				assert.Loosely(t, mustGetDimensions(t, jd).String(), should.Match(ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "A", Expiration: swSlice1Exp},
						{Value: "AA", Expiration: swSlice1Exp},
						{Value: "B", Expiration: swSlice2Exp},
						{Value: "C", Expiration: swSlice3Exp},
						{Value: "Z", Expiration: swSlice3Exp},
					},
				}.String()))

				if sw := jd.GetSwarming(); sw != nil {
					// ensure dimensions show up in ALL slices which they ought to.
					assert.Loosely(t, sw.Task.TaskSlices[0].Properties.Dimensions, should.Match([]*swarmingpb.StringPair{
						{
							Key:   "key",
							Value: "A",
						},
						{
							Key:   "key",
							Value: "AA",
						},
						{
							Key:   "key",
							Value: "B",
						},
						{
							Key:   "key",
							Value: "C",
						},
						{
							Key:   "key",
							Value: "Z",
						},
					}))
					assert.Loosely(t, sw.Task.TaskSlices[1].Properties.Dimensions, should.Match([]*swarmingpb.StringPair{
						{
							Key:   "key",
							Value: "B",
						},
						{
							Key:   "key",
							Value: "C",
						},
						{
							Key:   "key",
							Value: "Z",
						},
					}))
					assert.Loosely(t, sw.Task.TaskSlices[2].Properties.Dimensions, should.Match([]*swarmingpb.StringPair{
						{
							Key:   "key",
							Value: "C",
						},
						{
							Key:   "key",
							Value: "Z",
						},
					}))
				} else {
					rdims := jd.GetBuildbucket().BbagentArgs.Build.Infra.Swarming.TaskDimensions
					assert.Loosely(t, rdims, should.Match([]*bbpb.RequestedDimension{
						{Key: "key", Value: "A", Expiration: durationpb.New(swSlice1Exp)},
						{Key: "key", Value: "AA", Expiration: durationpb.New(swSlice1Exp)},
						{Key: "key", Value: "B", Expiration: durationpb.New(swSlice2Exp)},
						{Key: "key", Value: "C", Expiration: durationpb.New(swSlice3Exp)},
						{Key: "key", Value: "Z"},
					}))
				}
			},
		},

		{
			name:        "replace",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				baselineDims(t, jd)

				MustEdit(t, jd, func(je Editor) {
					je.SetDimensions(ExpiringDimensions{
						"key": []ExpiringValue{
							{Value: "norp", Expiration: swSlice1Exp},
						},
					})
				})

				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "norp", Expiration: swSlice1Exp},
					},
				}))
			},
		},

		{
			name:        "delete",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				baselineDims(t, jd)

				MustEdit(t, jd, func(je Editor) {
					je.SetDimensions(nil)
				})

				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(ExpiringDimensions{}))
			},
		},

		{
			name:        "bad expiration",
			skipSWEmpty: true,
			skipBB:      true,
			fn: func(t *ftt.Test, jd *Definition) {
				err := jd.Edit(func(je Editor) {
					je.SetDimensions(ExpiringDimensions{
						"key": []ExpiringValue{
							{Value: "narp", Expiration: time.Second * 10},
						},
					})
				})
				assert.Loosely(t, err, should.ErrLike(
					"key=narp@10 has invalid expiration time: "+
						"current slices expire at [0 60 240 600]"))
			},
		},
	})
}

func editDims(t testing.TB, jd *Definition, cmds ...string) {
	t.Helper()
	editCmds, err := MakeDimensionEditCommands(cmds)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	err = jd.Edit(func(je Editor) {
		je.EditDimensions(editCmds)
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
}

func TestEditDimensions(t *testing.T) {
	t.Parallel()

	runCases(t, "EditDimensions", []testCase{
		{
			name: "nil (empty)",
			fn: func(t *ftt.Test, jd *Definition) {
				editDims(t, jd) // no edit commands
				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(ExpiringDimensions{}))
			},
		},

		{
			name:        "nil (existing)",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				base := baselineDims(t, jd)
				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(base))

				editDims(t, jd) // no edit commands

				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(base))
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				editDims(t, jd,
					fmt.Sprintf("key+=value@%d", swSlice1ExpSecs),
					fmt.Sprintf("key+=other_value@%d", swSlice3ExpSecs),
					"other-=bogus",
					"reset=everything",
					"reset=else",
				)

				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "value", Expiration: swSlice1Exp},
						{Value: "other_value", Expiration: swSlice3Exp},
					},
					"reset": []ExpiringValue{
						{Value: "else", Expiration: swSlice3Exp},
						{Value: "everything", Expiration: swSlice3Exp},
					},
				}))

				if sw := jd.GetSwarming(); sw != nil {
					// ensure dimensions show up in ALL slices which they ought to.
					assert.Loosely(t, sw.Task.TaskSlices[0].Properties.Dimensions, should.Match([]*swarmingpb.StringPair{
						{
							Key:   "key",
							Value: "other_value",
						},
						{
							Key:   "key",
							Value: "value",
						},
						{
							Key:   "reset",
							Value: "else",
						},
						{
							Key:   "reset",
							Value: "everything",
						},
					}))
					assert.Loosely(t, sw.Task.TaskSlices[1].Properties.Dimensions, should.Match([]*swarmingpb.StringPair{
						{
							Key:   "key",
							Value: "other_value",
						},
						{
							Key:   "reset",
							Value: "else",
						},
						{
							Key:   "reset",
							Value: "everything",
						},
					}))
					assert.Loosely(t, sw.Task.TaskSlices[2].Properties.Dimensions, should.Match([]*swarmingpb.StringPair{
						{
							Key:   "key",
							Value: "other_value",
						},
						{
							Key:   "reset",
							Value: "else",
						},
						{
							Key:   "reset",
							Value: "everything",
						},
					}))
				} else {
					rdims := jd.GetBuildbucket().BbagentArgs.Build.Infra.Swarming.TaskDimensions
					assert.Loosely(t, rdims, should.Match([]*bbpb.RequestedDimension{
						{Key: "key", Value: "other_value", Expiration: durationpb.New(swSlice3Exp)},
						{Key: "key", Value: "value", Expiration: durationpb.New(swSlice1Exp)},
						{Key: "reset", Value: "else"},
						{Key: "reset", Value: "everything"},
					}))
				}
			},
		},

		{
			name:        "remove",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				editDims(t, jd,
					fmt.Sprintf("key+=value@%d", swSlice1ExpSecs),
					fmt.Sprintf("key+=other_value@%d", swSlice3ExpSecs),
					"reset=everything",
					"reset=else",
				)

				editDims(t, jd, "key-=other_value")

				assert.Loosely(t, mustGetDimensions(t, jd), should.Match(ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "value", Expiration: swSlice1Exp},
					},
					"reset": []ExpiringValue{
						{Value: "else", Expiration: swSlice3Exp},
						{Value: "everything", Expiration: swSlice3Exp},
					},
				}))
			},
		},
	})
}
