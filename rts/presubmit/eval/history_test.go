// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0(the "License");
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
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/sync/errgroup"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReadDurationData(t *testing.T) {
	t.Parallel()
	Convey("ReadDurationData", t, func() {
		ctx := context.Background()

		recordC := make(chan *evalpb.TestDurationRecord)
		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() error {
			defer close(recordC)
			return readTestDurations(ctx, filepath.Join("testdata", "durations"), recordC)
		})

		var records []*evalpb.TestDurationRecord
		eg.Go(func() error {
			for rec := range recordC {
				records = append(records, rec)
			}
			return nil
		})
		So(eg.Wait(), ShouldBeNil)

		So(records, ShouldHaveLength, 2)
		So(records[0], ShouldResembleProtoJSON, `{
			"patchsets": [
				{
					"change": {
						"host": "chromium-review.googlsource.com",
						"project": "src",
						"number": "2561024"
					},
					"patchset": "4",
					"changedFiles": [
						{
							"repo": "https://chromium.googlsource.com/src",
							"path": "//android_webview/browser/aw_contents.cc"
						},
						{
							"repo": "https://chromium.googlsource.com/src",
							"path": "//android_webview/browser/aw_settings.cc"
						}
					]
				}
			],
			"testDurations": [
				{
					"testVariant": {
						"id": "ninja://chrome/test:browser_tests/InterstitialUITest.InterstitialViewSource",
						"variant": [
							"builder:linux-rel",
							"os:Ubuntu-16.04",
							"test_suite:browser_tests"
						],
						"fileName": "//chrome/browser/ui/webui/interstitials/interstitial_ui_browsertest.cc"
					},
					"duration": "1.573000s"
				},
				{
					"testVariant": {
						"id": "ninja://chrome/test:browser_tests/LookalikeUrlNavigationThrottleBrowserTest.PunycodeAndTargetEmbedding_NoSuggestedUrl_Interstitial/All.3",
						"variant": [
							"builder:linux-rel",
							"os:Ubuntu-16.04",
							"test_suite:browser_tests"
						],
						"fileName": "//chrome/browser/lookalikes/lookalike_url_navigation_throttle_browsertest.cc"
					},
					"duration": "1.575000s"
				}
			]
		}`)
		So(records[1], ShouldResembleProtoJSON, `{
			"patchsets": [
				{
					"change": {
						"host": "chromium-review.googlsource.com",
						"project": "src",
						"number": "2424208"
					},
					"patchset": "42",
					"changedFiles": [
						{
							"repo": "https://chromium.googlsource.com/src",
							"path": "//chrome/browser/chrome_back_forward_cache_browsertest.cc"
						}
					]
				}
			],
			"testDurations": [
				{
					"testVariant": {
						"id": "ninja://content/test:content_browsertests/WebRtcBrowserTest.CanSetupVideoCallAndDisableLocalVideo",
						"variant": [
							"builder:linux-rel",
							"os:Ubuntu-16.04",
							"test_suite:content_browsertests"
						],
						"fileName": "//content/browser/webrtc/webrtc_browsertest.cc"
					},
					"duration": "2.096000s"
				}
			]
		}`)
	})
}
