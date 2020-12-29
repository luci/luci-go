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

package eval

import (
	"bytes"
	"strings"
	"testing"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPrintLostRejection(t *testing.T) {
	t.Parallel()

	assert := func(rej *evalpb.Rejection, expectedText string) {
		buf := &bytes.Buffer{}
		p := rejectionPrinter{printer: newPrinter(buf)}
		So(p.rejection(rej), ShouldBeNil)
		expectedText = strings.Replace(expectedText, "\t", "  ", -1)
		So(buf.String(), ShouldEqual, expectedText)
	}

	ps1 := &evalpb.GerritPatchset{
		Change: &evalpb.GerritChange{
			Host:    "chromium-review.googlesource.com",
			Project: "chromium/src",
			Number:  123,
		},
		Patchset: 4,
	}
	ps2 := &evalpb.GerritPatchset{
		Change: &evalpb.GerritChange{
			Host:    "chromium-review.googlesource.com",
			Project: "chromium/src",
			Number:  223,
		},
		Patchset: 4,
	}

	Convey(`PrintLostRejection`, t, func() {
		Convey(`Basic`, func() {
			rej := &evalpb.Rejection{
				Patchsets:          []*evalpb.GerritPatchset{ps1},
				FailedTestVariants: []*evalpb.TestVariant{{Id: "test1"}},
			}

			assert(rej, `Lost rejection:
	https://chromium-review.googlesource.com/c/123/4
	Failed and not selected tests:
		- <empty test variant>
			- test1
`)
		})

		Convey(`With file name`, func() {
			rej := &evalpb.Rejection{
				Patchsets: []*evalpb.GerritPatchset{ps1},
				FailedTestVariants: []*evalpb.TestVariant{{
					Id:       "test1",
					FileName: "test.cc",
				}},
			}

			assert(rej, `Lost rejection:
	https://chromium-review.googlesource.com/c/123/4
	Failed and not selected tests:
		- <empty test variant>
			- test1
				in test.cc
`)
		})

		Convey(`Multiple variants`, func() {
			rej := &evalpb.Rejection{
				Patchsets: []*evalpb.GerritPatchset{ps1},
				FailedTestVariants: []*evalpb.TestVariant{
					{
						Id: "test1",
						Variant: map[string]string{
							"a": "0",
						},
					},
					{
						Id: "test2",
						Variant: map[string]string{
							"a": "0",
						},
					},
					{
						Id: "test1",
						Variant: map[string]string{
							"a": "0",
							"b": "0",
						},
					},
				},
			}

			assert(rej, `Lost rejection:
	https://chromium-review.googlesource.com/c/123/4
	Failed and not selected tests:
		- a:0
			- test1
			- test2
		- a:0 | b:0
			- test1
`)
		})

		Convey(`Two patchsets`, func() {
			rej := &evalpb.Rejection{
				Patchsets:          []*evalpb.GerritPatchset{ps1, ps2},
				FailedTestVariants: []*evalpb.TestVariant{{Id: "test1"}},
			}

			assert(rej, `Lost rejection:
	- patchsets:
		https://chromium-review.googlesource.com/c/123/4
		https://chromium-review.googlesource.com/c/223/4
	Failed and not selected tests:
		- <empty test variant>
			- test1
`)
		})
	})
}
