// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func load(name string) ([]byte, error) {
	filename := strings.Join([]string{"expectations", name}, "/")
	return ioutil.ReadFile(filename)
}

func shouldMatchExpectationsFor(actualContents interface{}, expectedFilename ...interface{}) string {
	buildJSON, err := json.Marshal(actualContents)
	if err != nil {
		return fmt.Sprintf("Could not marshal %v", buildJSON)
	}
	refBuild, err := load(expectedFilename[0].(string))
	if err != nil {
		return fmt.Sprintf("Could not load %s: %s", expectedFilename[0], err.Error())
	}
	refBuildJSON := strings.TrimSpace(string(refBuild))
	return ShouldEqual(string(buildJSON), refBuildJSON)
}

func TestBuild(t *testing.T) {
	Convey(`A test Environment`, t, func() {
		c := context.Background()
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

		Convey(`Build Impl`, func() {
			build, err := swarmingBuildImpl(c, "foo", "default", "debug:build-patch-failure")
			So(err, ShouldBeNil)
			So(build, shouldMatchExpectationsFor, "build-patch-failure.json")
		})
	})
}
