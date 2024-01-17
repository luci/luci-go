// Copyright 2021 The LUCI Authors.
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

package cvtesting

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/cv/internal/cvtesting/saferesembletest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSafeShouldResemble(t *testing.T) {
	t.Parallel()

	// pbShuffle returns unmarshal(marshal(t)).
	//
	// This exists to test the case on which the original Go Convey's
	// ShouldResemble (and its friends) hang, e.g. the following hangs:
	//   ShouldResemble(t, pbShuffle(t))
	pbShuffle := func(t proto.Message) proto.Message {
		out, err := proto.Marshal(t)
		if err != nil {
			panic(err)
		}
		ret := reflect.New(reflect.TypeOf(t).Elem()).Interface().(proto.Message)
		if err = proto.Unmarshal(out, ret); err != nil {
			panic(err)
		}
		return ret
	}

	var (
		ts66        = &timestamppb.Timestamp{Seconds: 6, Nanos: 6}
		ts53        = &timestamppb.Timestamp{Seconds: 5, Nanos: 3}
		ts53duped   = &timestamppb.Timestamp{Seconds: 5, Nanos: 3}
		ts53decoded = pbShuffle(ts53).(*timestamppb.Timestamp)

		d22        = &durationpb.Duration{Seconds: 2, Nanos: 2}
		d43        = &durationpb.Duration{Seconds: 5, Nanos: 3}
		d43cloned  = proto.Clone(d43).(*durationpb.Duration)
		d43decoded = pbShuffle(d43).(*durationpb.Duration)
	)

	cases := []struct {
		name            string
		a, e            any
		diffsContains   []string
		shouldPanicLike string
	}{
		// Nil special cases.
		{
			name: "nils are equal",
			a:    (*tNoProtos)(nil),
			e:    (*tNoProtos)(nil),
		},
		{
			name:          "nil vs non-nil",
			a:             (*tNoProtos)(nil),
			e:             &tNoProtos{},
			diffsContains: []string{"actual is nil, but not nil is expected"},
		},
		{
			name:          "non-nil vs nil",
			a:             &tNoProtos{},
			e:             (*tNoProtos)(nil),
			diffsContains: []string{"actual is not nil, but nil is expected"},
		},

		// Equality of zero values.
		{
			name: "equal zero value without protos",
			a:    tNoProtos{},
			e:    tNoProtos{},
		},
		{
			name: "equal zero value with protos",
			a:    tWith2Protos{},
			e:    tWith2Protos{},
		},
		{
			name: "equal ptr to zero value without protos",
			a:    &tNoProtos{},
			e:    &tNoProtos{},
		},
		{
			name: "equal ptr to zero value with protos",
			a:    &tWith2Protos{},
			e:    &tWith2Protos{},
		},

		// Equality with populated fields.
		{
			name: "equal without protos",
			a:    tNoProtos{a: 1, A: testclock.TestRecentTimeUTC},
			e:    tNoProtos{a: 1, A: testclock.TestRecentTimeUTC},
		},
		{
			name: "equal with proto as same ptr",
			a:    tWithProto{a: 1, PB: ts53},
			e:    tWithProto{a: 1, PB: ts53},
		},
		{
			name: "equal with proto same value",
			a:    tWithProto{a: 1, PB: ts53},
			e:    tWithProto{a: 1, PB: ts53duped},
		},
		{
			name: "equal with proto with IO",
			a:    tWithProto{a: 1, PB: ts53},
			e:    tWithProto{a: 1, PB: ts53decoded},
		},
		{
			name: "equal with proto cloned",
			a:    tWith2Protos{A: 1, b: "b", T: ts53, D: d43},
			e:    tWith2Protos{A: 1, b: "b", T: ts53, D: d43cloned},
		},
		{
			name: "equal with private fields and no protos",
			a:    saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
			e:    saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
		},
		{
			name: "equal with private fields and private protos",
			a:    saferesembletest.NewWithPrivateProto(1, "b", ts53),
			e:    saferesembletest.NewWithPrivateProto(1, "b", ts53decoded),
		},
		{
			name: "equal with inner struct fields and no protos",
			a:    saferesembletest.NewWithInnerStructNoProto(2, "y", saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC)),
			e:    saferesembletest.NewWithInnerStructNoProto(2, "y", saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC)),
		},
		{
			name: "equal with inner struct fields and has proto",
			a:    saferesembletest.NewWithInnerStructHasProto(2, "y", saferesembletest.NewWithPrivateProto(1, "b", ts53)),
			e:    saferesembletest.NewWithInnerStructHasProto(2, "y", saferesembletest.NewWithPrivateProto(1, "b", ts53decoded)),
		},
		{
			name: "equal with private inner struct fields and no protos",
			a:    saferesembletest.NewWithPrivateInnerStructNoProto(2, "y", saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC)),
			e:    saferesembletest.NewWithPrivateInnerStructNoProto(2, "y", saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC)),
		},
		{
			name: "equal with private inner struct fields and has proto",
			a:    saferesembletest.NewWithPrivateStructHasProto(2, "y", saferesembletest.NewWithPrivateProto(1, "b", ts53)),
			e:    saferesembletest.NewWithPrivateStructHasProto(2, "y", saferesembletest.NewWithPrivateProto(1, "b", ts53decoded)),
		},
		{
			name: "equal with inner struct slice field and no protos",
			a: saferesembletest.NewWithInnerStructSliceNoProto(2, "y", []saferesembletest.NoProtos{
				saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
			}),
			e: saferesembletest.NewWithInnerStructSliceNoProto(2, "y", []saferesembletest.NoProtos{
				saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
			}),
		},
		{
			name: "equal with inner struct slice field and has proto",
			a: saferesembletest.NewWithInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53),
			}),
			e: saferesembletest.NewWithInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53decoded),
			}),
		},
		{
			name: "equal with private inner struct slice fields and no protos",
			a: saferesembletest.NewWithPrivateInnerStructSliceNoProto(2, "y", []saferesembletest.NoProtos{
				saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
			}),
			e: saferesembletest.NewWithPrivateInnerStructSliceNoProto(2, "y", []saferesembletest.NoProtos{
				saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
			}),
		},
		{
			name: "equal with private inner struct fields slice and has proto",
			a: saferesembletest.NewWithPrivateInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53),
			}),
			e: saferesembletest.NewWithPrivateInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53decoded),
			}),
		},

		// Diff.
		{
			name:          "diff without no protos",
			a:             &tNoProtos{a: 1},
			e:             &tNoProtos{a: 2},
			diffsContains: []string{"non-proto fields differ:", "(Should equal)!"},
		},
		{
			name:          "diff, but not in protos",
			a:             &tWith2Protos{A: 123, T: ts53, D: d43},
			e:             &tWith2Protos{b: "B", T: ts53, D: d43decoded},
			diffsContains: []string{"non-proto fields differ:", "123", "B"},
		},
		{
			name:          "diff only in 1 proto",
			a:             &tWith2Protos{b: "B", T: ts53, D: d43},
			e:             &tWith2Protos{b: "B", T: ts53, D: d22},
			diffsContains: []string{"field .D differs:"},
		},
		{
			name:          "diff only in 2 protos",
			a:             &tWith2Protos{b: "B", T: ts66, D: d43},
			e:             &tWith2Protos{b: "B", T: ts53, D: d22},
			diffsContains: []string{"field .T differs:", "field .D differs:"},
		},
		{
			name:          "diff everywhere",
			a:             &tWith2Protos{A: 111, b: "bbb", T: ts66, D: d43},
			e:             &tWith2Protos{A: 222, b: "BBB", T: ts53, D: d22},
			diffsContains: []string{"111", "BBB", "field .T differs:", "field .D differs:"},
		},
		{
			name:          "diff everywhere with private fields & protos",
			a:             saferesembletest.NewWithPrivateProto(111, "bbb", ts53),
			e:             saferesembletest.NewWithPrivateProto(222, "BBB", ts66),
			diffsContains: []string{"111", "BBB", "field .t differs:", "non-proto fields differ:"},
		},
		{
			name:          "diff in 1 proto element of a slice",
			a:             &tWithProtoSlice{TS: []*timestamppb.Timestamp{ts53duped, ts53}},
			e:             &tWithProtoSlice{TS: []*timestamppb.Timestamp{ts53decoded, ts66}},
			diffsContains: []string{"field .TS differs:"},
		},
		{
			name:          "diff inner struct no protos",
			a:             saferesembletest.NewWithInnerStructNoProto(2, "y", saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC)),
			e:             saferesembletest.NewWithInnerStructNoProto(2, "y", saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC.Add(1*time.Second))),
			diffsContains: []string{"non-proto fields differ:", "(Should equal)!"},
		},
		{
			name:          "diff inner struct has proto",
			a:             saferesembletest.NewWithInnerStructHasProto(2, "y", saferesembletest.NewWithPrivateProto(1, "b", ts66)),
			e:             saferesembletest.NewWithInnerStructHasProto(2, "y", saferesembletest.NewWithPrivateProto(1, "b", ts53)),
			diffsContains: []string{"field .I.t differs:"},
		},
		{
			name: "diff inner struct slice no protos",
			a: saferesembletest.NewWithInnerStructSliceNoProto(2, "y", []saferesembletest.NoProtos{
				saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC),
			}),
			e: saferesembletest.NewWithInnerStructSliceNoProto(2, "y", []saferesembletest.NoProtos{
				saferesembletest.NewNoProtos(1, testclock.TestRecentTimeUTC.Add(1*time.Second)),
			}),
			diffsContains: []string{"non-proto fields differ:", "(Should equal)!"},
		},
		{
			name: "diff inner struct slice has proto",
			a: saferesembletest.NewWithInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts66),
			}),
			e: saferesembletest.NewWithInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53),
			}),
			diffsContains: []string{"field .Is[0].t differs:"},
		},
		{
			name: "diff in struct slice length of a slice",
			a: saferesembletest.NewWithInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53),
				saferesembletest.NewWithPrivateProto(10, "bb", ts53),
			}),
			e: saferesembletest.NewWithInnerStructSliceHasProto(2, "y", []saferesembletest.WithPrivateProto{
				saferesembletest.NewWithPrivateProto(1, "b", ts53),
			}),
			diffsContains: []string{"field .Is differs in length:"},
		},
	}

	for i, c := range cases {
		i, tCase := i, c
		name := fmt.Sprintf("%2d: %s", i, tCase.name)
		Convey(name, t, func() {
			if tCase.shouldPanicLike != "" {
				So(func() { SafeShouldResemble(tCase.a, tCase.e) }, assertions.ShouldPanicLike, tCase.shouldPanicLike)
				return
			}

			diff := SafeShouldResemble(tCase.a, tCase.e)
			if len(tCase.diffsContains) == 0 {
				So(diff, ShouldEqual, "")
				return
			}
			_, _ = Printf("\n\n===== diff emitted for %s =====\n%s\n%s\n", name, diff, strings.Repeat("=", 80))
			for _, sub := range tCase.diffsContains {
				So(diff, ShouldContainSubstring, sub)
			}
		})
	}
}

type tNoProtos struct {
	a int
	A time.Time
}

type tWithProto struct {
	a  int
	PB *timestamppb.Timestamp
}

type tWith2Protos struct {
	A int
	b string
	T *timestamppb.Timestamp
	D *durationpb.Duration
}

type tWithProtoSlice struct {
	TS []*timestamppb.Timestamp
}
