// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"

	"github.com/luci/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func load(name string) ([]byte, error) {
	filename := filepath.Join("expectations", name)
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
	t.Parallel()

	if *generate {
		c := context.Background()
		c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))
		c = memory.Use(c)

		for _, tc := range getTestCases() {
			bl := buildLoader{
				logDogClientFunc: logDogClientFunc(tc),
			}
			svc := debugSwarmingService{tc}

			fmt.Printf("Generating expectations for %s\n", tc)

			build, err := bl.swarmingBuildImpl(c, svc, "foo", tc.name)
			if err != nil {
				panic(fmt.Errorf("Could not run swarmingBuildImpl for %s: %s", tc, err))
			}
			buildJSON, err := json.MarshalIndent(build, "", "  ")
			if err != nil {
				panic(fmt.Errorf("Could not JSON marshal %s: %s", tc.name, err))
			}
			filename := filepath.Join("expectations", tc.name+".json")
			err = ioutil.WriteFile(filename, []byte(buildJSON), 0644)
			if err != nil {
				panic(fmt.Errorf("Encountered error while trying to write to %s: %s", filename, err))
			}
		}
		return
	}

	Convey(`A test Environment`, t, func() {
		c := context.Background()
		c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))
		c = memory.Use(c)

		for _, tc := range getTestCases() {
			Convey(fmt.Sprintf("Test Case: %s", tc.name), func() {
				bl := buildLoader{
					logDogClientFunc: logDogClientFunc(tc),
				}
				svc := debugSwarmingService{tc}

				build, err := bl.swarmingBuildImpl(c, svc, "foo", tc.name)
				So(err, ShouldBeNil)
				So(build, shouldMatchExpectationsFor, tc.name+".json")
			})
		}
	})
}
