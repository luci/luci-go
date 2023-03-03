// Copyright 2023 The LUCI Authors.
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
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeAndDecode(t *testing.T) {
	Convey(`Encode and decode should return the same result`, t, func() {
		history := History{
			Verdicts: []PositionVerdict{
				{
					CommitPosition:   1345,
					IsSimpleExpected: true,
					Hour:             time.Unix(1000*3600, 0),
				},
				{
					CommitPosition:   1355,
					IsSimpleExpected: false,
					Hour:             time.Unix(1005*3600, 0),
					Details: VerdictDetails{
						IsExonerated: false,
						Runs: []Run{
							{
								ExpectedResultCount:   1,
								UnexpectedResultCount: 2,
								IsDuplicate:           false,
							},
							{
								ExpectedResultCount:   2,
								UnexpectedResultCount: 3,
								IsDuplicate:           true,
							},
						},
					},
				},
				{
					CommitPosition:   1357,
					IsSimpleExpected: true,
					Hour:             time.Unix(1003*3600, 0),
				},
				{
					CommitPosition:   1357,
					IsSimpleExpected: false,
					Hour:             time.Unix(1005*3600, 0),
					Details: VerdictDetails{
						IsExonerated: true,
						Runs: []Run{
							{
								ExpectedResultCount:   0,
								UnexpectedResultCount: 1,
								IsDuplicate:           true,
							},
							{
								ExpectedResultCount:   0,
								UnexpectedResultCount: 1,
								IsDuplicate:           false,
							},
						},
					},
				},
			},
		}

		encoded := EncodeHistory(history)
		decodedHistory, err := DecodeHistory(encoded)
		So(err, ShouldBeNil)
		So(len(decodedHistory.Verdicts), ShouldEqual, 4)
		So(decodedHistory, ShouldResemble, history)
	})

	Convey(`Encode and decode long history should not have error`, t, func() {
		history := History{}
		history.Verdicts = make([]PositionVerdict, 2000)
		for i := 0; i < 2000; i++ {
			history.Verdicts[i] = PositionVerdict{
				CommitPosition:   i,
				IsSimpleExpected: false,
				Hour:             time.Unix(int64(i*3600), 0),
				Details: VerdictDetails{
					IsExonerated: false,
					Runs: []Run{
						{
							ExpectedResultCount:   1,
							UnexpectedResultCount: 2,
							IsDuplicate:           false,
						},
						{
							ExpectedResultCount:   1,
							UnexpectedResultCount: 2,
							IsDuplicate:           false,
						},
						{
							ExpectedResultCount:   1,
							UnexpectedResultCount: 2,
							IsDuplicate:           false,
						},
					},
				},
			}
		}
		encoded := EncodeHistory(history)
		decodedHistory, err := DecodeHistory(encoded)
		So(err, ShouldBeNil)
		So(len(decodedHistory.Verdicts), ShouldEqual, 2000)
		So(decodedHistory, ShouldResemble, history)
	})
}
