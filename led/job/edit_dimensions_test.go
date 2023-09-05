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
	fmt "fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	api "go.chromium.org/luci/swarming/proto/api"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey(`MakeDimensionEditCommands`, t, func() {
		for _, tc := range testCases {
			tc := tc
			Convey(tc.name, func() {
				dec, err := MakeDimensionEditCommands(tc.cmds)
				if tc.err == "" {
					So(err, ShouldBeNil)
					So(dec, ShouldResemble, tc.expect)
				} else {
					So(err, ShouldErrLike, tc.err)
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
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.SetDimensions(nil)
				})
				So(mustGetDimensions(jd), ShouldBeEmpty)
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				baselineDims(jd)

				So(mustGetDimensions(jd).String(), ShouldResemble, ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "A", Expiration: swSlice1Exp},
						{Value: "AA", Expiration: swSlice1Exp},
						{Value: "B", Expiration: swSlice2Exp},
						{Value: "C", Expiration: swSlice3Exp},
						{Value: "Z", Expiration: swSlice3Exp},
					},
				}.String())

				if sw := jd.GetSwarming(); sw != nil {
					// ensure dimensions show up in ALL slices which they ought to.
					So(sw.Task.TaskSlices[0].Properties.Dimensions, ShouldResembleProto, []*api.StringListPair{
						{
							Key:    "key",
							Values: []string{"A", "AA", "B", "C", "Z"},
						},
					})
					So(sw.Task.TaskSlices[1].Properties.Dimensions, ShouldResembleProto, []*api.StringListPair{
						{
							Key:    "key",
							Values: []string{"B", "C", "Z"},
						},
					})
					So(sw.Task.TaskSlices[2].Properties.Dimensions, ShouldResembleProto, []*api.StringListPair{
						{
							Key:    "key",
							Values: []string{"C", "Z"},
						},
					})
				} else {
					rdims := jd.GetBuildbucket().BbagentArgs.Build.Infra.Swarming.TaskDimensions
					So(rdims, ShouldResembleProto, []*bbpb.RequestedDimension{
						{Key: "key", Value: "A", Expiration: durationpb.New(swSlice1Exp)},
						{Key: "key", Value: "AA", Expiration: durationpb.New(swSlice1Exp)},
						{Key: "key", Value: "B", Expiration: durationpb.New(swSlice2Exp)},
						{Key: "key", Value: "C", Expiration: durationpb.New(swSlice3Exp)},
						{Key: "key", Value: "Z"},
					})
				}
			},
		},

		{
			name:        "replace",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				baselineDims(jd)

				SoEdit(jd, func(je Editor) {
					je.SetDimensions(ExpiringDimensions{
						"key": []ExpiringValue{
							{Value: "norp", Expiration: swSlice1Exp},
						},
					})
				})

				So(mustGetDimensions(jd), ShouldResemble, ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "norp", Expiration: swSlice1Exp},
					},
				})
			},
		},

		{
			name:        "delete",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				baselineDims(jd)

				SoEdit(jd, func(je Editor) {
					je.SetDimensions(nil)
				})

				So(mustGetDimensions(jd), ShouldResemble, ExpiringDimensions{})
			},
		},

		{
			name:        "bad expiration",
			skipSWEmpty: true,
			skipBB:      true,
			fn: func(jd *Definition) {
				err := jd.Edit(func(je Editor) {
					je.SetDimensions(ExpiringDimensions{
						"key": []ExpiringValue{
							{Value: "narp", Expiration: time.Second * 10},
						},
					})
				})
				So(err, ShouldErrLike,
					"key=narp@10 has invalid expiration time: "+
						"current slices expire at [0 60 240 600]")
			},
		},
	})

}

func editDims(jd *Definition, cmds ...string) {
	editCmds, err := MakeDimensionEditCommands(cmds)
	So(err, ShouldBeNil)
	err = jd.Edit(func(je Editor) {
		je.EditDimensions(editCmds)
	})
	So(err, ShouldBeNil)
}

func TestEditDimensions(t *testing.T) {
	t.Parallel()

	runCases(t, "EditDimensions", []testCase{
		{
			name: "nil (empty)",
			fn: func(jd *Definition) {
				editDims(jd) // no edit commands
				So(mustGetDimensions(jd), ShouldResemble, ExpiringDimensions{})
			},
		},

		{
			name:        "nil (existing)",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				base := baselineDims(jd)
				So(mustGetDimensions(jd), ShouldResemble, base)

				editDims(jd) // no edit commands

				So(mustGetDimensions(jd), ShouldResemble, base)
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				editDims(jd,
					fmt.Sprintf("key+=value@%d", swSlice1ExpSecs),
					fmt.Sprintf("key+=other_value@%d", swSlice3ExpSecs),
					"other-=bogus",
					"reset=everything",
					"reset=else",
				)

				So(mustGetDimensions(jd), ShouldResemble, ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "value", Expiration: swSlice1Exp},
						{Value: "other_value", Expiration: swSlice3Exp},
					},
					"reset": []ExpiringValue{
						{Value: "else", Expiration: swSlice3Exp},
						{Value: "everything", Expiration: swSlice3Exp},
					},
				})

				if sw := jd.GetSwarming(); sw != nil {
					// ensure dimensions show up in ALL slices which they ought to.
					So(sw.Task.TaskSlices[0].Properties.Dimensions, ShouldResembleProto, []*api.StringListPair{
						{
							Key:    "key",
							Values: []string{"other_value", "value"},
						},
						{
							Key:    "reset",
							Values: []string{"else", "everything"},
						},
					})
					So(sw.Task.TaskSlices[1].Properties.Dimensions, ShouldResembleProto, []*api.StringListPair{
						{
							Key:    "key",
							Values: []string{"other_value"},
						},
						{
							Key:    "reset",
							Values: []string{"else", "everything"},
						},
					})
					So(sw.Task.TaskSlices[2].Properties.Dimensions, ShouldResembleProto, []*api.StringListPair{
						{
							Key:    "key",
							Values: []string{"other_value"},
						},
						{
							Key:    "reset",
							Values: []string{"else", "everything"},
						},
					})
				} else {
					rdims := jd.GetBuildbucket().BbagentArgs.Build.Infra.Swarming.TaskDimensions
					So(rdims, ShouldResembleProto, []*bbpb.RequestedDimension{
						{Key: "key", Value: "other_value", Expiration: durationpb.New(swSlice3Exp)},
						{Key: "key", Value: "value", Expiration: durationpb.New(swSlice1Exp)},
						{Key: "reset", Value: "else"},
						{Key: "reset", Value: "everything"},
					})
				}
			},
		},

		{
			name:        "remove",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				editDims(jd,
					fmt.Sprintf("key+=value@%d", swSlice1ExpSecs),
					fmt.Sprintf("key+=other_value@%d", swSlice3ExpSecs),
					"reset=everything",
					"reset=else",
				)

				editDims(jd, "key-=other_value")

				So(mustGetDimensions(jd), ShouldResemble, ExpiringDimensions{
					"key": []ExpiringValue{
						{Value: "value", Expiration: swSlice1Exp},
					},
					"reset": []ExpiringValue{
						{Value: "else", Expiration: swSlice3Exp},
						{Value: "everything", Expiration: swSlice3Exp},
					},
				})

			},
		},
	})
}
