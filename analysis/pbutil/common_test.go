// Copyright 2022 The LUCI Authors.
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

	"go.chromium.org/luci/cv/api/bigquery/v1"
	cvv0 "go.chromium.org/luci/cv/api/v0"

	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCommon(t *testing.T) {
	Convey("PresubmitRunModeFromString", t, func() {
		// Confirm a mapping exists for every mode defined by LUCI CV.
		// This test is designed to break if LUCI CV extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.

		// NEW_PATCHSET_RUN and CQ_MODE_MEGA_DRY_RUN is not defined in the
		// LUCI CV BigQuery schema but may be returned by the RPC.
		modes := []string{"NEW_PATCHSET_RUN", "CQ_MODE_MEGA_DRY_RUN"}
		for _, mode := range bigquery.Mode_name {
			modes = append(modes, mode)
		}
		for _, mode := range bigquery.Mode_name {
			if mode == "MODE_UNSPECIFIED" {
				continue
			}
			mode, err := PresubmitRunModeFromString(mode)
			So(err, ShouldBeNil)
			So(mode, ShouldNotEqual, pb.PresubmitRunMode_PRESUBMIT_RUN_MODE_UNSPECIFIED)
		}
	})
	Convey("PresubmitRunStatusFromLUCICV", t, func() {
		// Confirm a mapping exists for every run status defined by LUCI CV.
		// This test is designed to break if LUCI CV extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range cvv0.Run_Status_value {
			runStatus := cvv0.Run_Status(v)
			if runStatus&cvv0.Run_ENDED_MASK == 0 {
				// Not a run ended status. LUCI Analysis should not have to
				// deal with these, as LUCI Analysis only ingests completed
				// runs.
				continue
			}
			if runStatus == cvv0.Run_ENDED_MASK {
				// The run ended mask is itself not a valid status.
				continue
			}
			status, err := PresubmitRunStatusFromLUCICV(runStatus)
			So(err, ShouldBeNil)
			So(status, ShouldNotEqual, pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_UNSPECIFIED)
		}
	})
}

func TestValidateStringPair(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateStringPairs`, t, func() {
		Convey(`empty`, func() {
			err := ValidateStringPair(StringPair("", ""))
			So(err, ShouldErrLike, `key: unspecified`)
		})

		Convey(`invalid key`, func() {
			err := ValidateStringPair(StringPair("1", ""))
			So(err, ShouldErrLike, `key: does not match`)
		})

		Convey(`long key`, func() {
			err := ValidateStringPair(StringPair(strings.Repeat("a", 1000), ""))
			So(err, ShouldErrLike, `key length must be less or equal to 64`)
		})

		Convey(`long value`, func() {
			err := ValidateStringPair(StringPair("a", strings.Repeat("a", 1000)))
			So(err, ShouldErrLike, `value length must be less or equal to 256`)
		})

		Convey(`multiline value`, func() {
			err := ValidateStringPair(StringPair("a", "multi\nline\nvalue"))
			So(err, ShouldBeNil)
		})

		Convey(`valid`, func() {
			err := ValidateStringPair(StringPair("a", "b"))
			So(err, ShouldBeNil)
		})
	})
}

func TestVariantToJSON(t *testing.T) {
	t.Parallel()
	Convey(`VariantToJSON`, t, func() {
		Convey(`empty`, func() {
			result, err := VariantToJSON(nil)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "{}")
		})
		Convey(`non-empty`, func() {
			variant := &pb.Variant{
				Def: map[string]string{
					"builder":           "linux-rel",
					"os":                "Ubuntu-18.04",
					"pathological-case": "\000\001\n\r\f",
				},
			}
			result, err := VariantToJSON(variant)
			So(err, ShouldBeNil)
			So(result, ShouldEqual, `{"builder":"linux-rel","os":"Ubuntu-18.04","pathological-case":"\u0000\u0001\n\r\u000c"}`)
		})
	})
}
