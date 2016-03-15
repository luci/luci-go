// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"testing"

	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func load(name string) ([]byte, error) {
	filename := strings.Join([]string{"expectations", name}, "/")
	return ioutil.ReadFile(filename)
}

func shouldMatchExpectationsFor(actualContents interface{}, expectedFilename ...interface{}) string {
	refBuild, err := load(expectedFilename[0].(string))
	if err != nil {
		return fmt.Sprintf("Could not load %s: %s", expectedFilename[0], err.Error())
	}
	actualBuild, err := json.MarshalIndent(actualContents, "", "  ")
	return ShouldEqual(string(actualBuild), string(refBuild))

}

func TestBuild(t *testing.T) {
	testCases := []struct {
		input        string
		expectations string
	}{
		{"debug:build-patch-failure", "build-patch-failure.json"},
		{"debug:build-exception", "build-exception.json"},
		{"debug:build-timeout", "build-timeout.json"},
	}

	if *generate {
		for _, tc := range testCases {
			c := context.Background()
			c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
			fmt.Printf("Generating expectations for %s\n", tc.input)
			build, err := swarmingBuildImpl(c, "foo", "default", tc.input)
			if err != nil {
				panic(fmt.Errorf("Could not run swarmingBuildImpl for %s: %s", tc.input, err))
			}
			buildJSON, err := json.MarshalIndent(build, "", "  ")
			if err != nil {
				panic(fmt.Errorf("Could not JSON marshal %s: %s", tc.input, err))
			}
			filename := path.Join("expectations", tc.expectations)
			err = ioutil.WriteFile(filename, []byte(buildJSON), 0644)
			if err != nil {
				panic(fmt.Errorf("Encountered error while trying to write to %s: %s", filename, err))
			}
		}
		return
	}

	Convey(`A test Environment`, t, func() {
		c := context.Background()
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

		for _, tc := range testCases {
			Convey(fmt.Sprintf("Test Case: %s", tc.input), func() {
				build, err := swarmingBuildImpl(c, "foo", "default", tc.input)
				So(err, ShouldBeNil)
				So(build, shouldMatchExpectationsFor, tc.expectations)
			})
		}
	})
}
