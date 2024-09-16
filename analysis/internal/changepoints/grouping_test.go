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

package changepoints

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGroupChangepoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	Convey("TestGroupChangepoints", t, func() {
		// Group 1 - test id num gap less than the TestIDGroupingThreshold in the same group.
		cp1 := makeChangepointRow(4, 100, 200, "refhash")
		cp2 := makeChangepointRow(30, 160, 261, "refhash") // 41 commits, 40.6% overlap with cp1
		cp3 := makeChangepointRow(90, 100, 200, "refhash")
		cp4 := makeChangepointRow(1, 100, 300, "refhash") // large regression range.
		// Group 2 - same test id group, but different regression range with group 1.
		cp5 := makeChangepointRow(2, 161, 263, "refhash") // 40 commits. 39.6% overlap wtih cp 1.
		cp6 := makeChangepointRow(3, 161, 264, "refhash")
		// Group 4 - different id group
		cp7 := makeChangepointRow(1000, 100, 200, "refhash")
		cp8 := makeChangepointRow(1001, 100, 200, "refhash")
		// Group 5 - same test variant can't be in the same group more than once.
		cp9 := makeChangepointRow(1001, 120, 230, "refhash")
		// Group 6 - different branch should't be grouped together.
		cp10 := makeChangepointRow(1002, 120, 230, "otherrefhash")

		groups := GroupChangepoints(ctx, []*ChangepointRow{cp1, cp2, cp3, cp4, cp5, cp6, cp7, cp8, cp9, cp10})
		So(groups, ShouldResemble, [][]*ChangepointRow{
			{cp1, cp3, cp2, cp4},
			{cp5, cp6},
			{cp7, cp8},
			{cp9},
			{cp10},
		})
	})
}

func TestStartOfWeek(t *testing.T) {
	ftt.Run("TestStartOfWeek", t, func(t *ftt.Test) {

		week := StartOfWeek(time.Date(2024, 9, 14, 0, 0, 0, 0, time.UTC))
		expected := time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC)
		assert.That(t, week, should.Match(expected))

		week = StartOfWeek(time.Date(2024, 9, 13, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = StartOfWeek(time.Date(2024, 9, 12, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = StartOfWeek(time.Date(2024, 9, 11, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = StartOfWeek(time.Date(2024, 9, 10, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = StartOfWeek(time.Date(2024, 9, 9, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = StartOfWeek(time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

	})
}

func makeChangepointRow(TestIDNum, lowerBound, upperBound int64, refHash string) *ChangepointRow {
	return &ChangepointRow{
		Project:     "chromium",
		TestIDNum:   TestIDNum,
		TestID:      fmt.Sprintf("test%d", TestIDNum),
		VariantHash: "varianthash",
		Ref: &Ref{
			Gitiles: &Gitiles{
				Host:    bigquery.NullString{Valid: true, StringVal: "host"},
				Project: bigquery.NullString{Valid: true, StringVal: "project"},
				Ref:     bigquery.NullString{Valid: true, StringVal: "ref"},
			},
		},
		RefHash:                      refHash,
		UnexpectedVerdictRateCurrent: 0,
		UnexpectedVerdictRateAfter:   0.99,
		UnexpectedVerdictRateBefore:  0.3,
		StartHour:                    time.Unix(1000, 0),
		LowerBound99th:               lowerBound,
		UpperBound99th:               upperBound,
		NominalStartPosition:         (lowerBound + upperBound) / 2,
	}
}
