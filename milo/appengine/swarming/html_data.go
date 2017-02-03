// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/settings"
	"github.com/luci/luci-go/server/templates"
)

var testCases = []struct {
	input        string
	expectations string
}{
	{"build-canceled", "build-canceled.json"},
	{"build-exception", "build-exception.json"},
	{"build-expired", "build-expired.json"},
	{"build-link", "build-link.json"},
	{"build-patch-failure", "build-patch-failure.json"},
	{"build-pending", "build-pending.json"},
	{"build-running", "build-running.json"},
	{"build-timeout", "build-timeout.json"},
	{"build-unicode", "build-unicode.json"},
	{"build-nested", "build-nested.json"},
}

func getTestCases() []string {
	// References "milo/appengine/swarming/testdata".
	testdata := filepath.Join("..", "swarming", "testdata")
	results := []string{}
	f, err := ioutil.ReadDir(testdata)
	if err != nil {
		panic(err)
	}
	for _, fi := range f {
		if strings.HasSuffix(fi.Name(), ".swarm") {
			results = append(results, fi.Name()[0:len(fi.Name())-6])
		}
	}
	return results
}

type debugSwarmingService struct{}

func (svc debugSwarmingService) getHost() string { return "example.com" }

func (svc debugSwarmingService) getContent(taskID, suffix string) ([]byte, error) {
	// ../swarming below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	logFilename := filepath.Join("..", "swarming", "testdata", taskID)
	if suffix != "" {
		logFilename += suffix
	}
	return ioutil.ReadFile(logFilename)
}

func (svc debugSwarmingService) getSwarmingJSON(taskID, suffix string, dst interface{}) error {
	content, err := svc.getContent(taskID, suffix)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, dst)
}

func (svc debugSwarmingService) getSwarmingResult(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskResult, error) {

	var sr swarming.SwarmingRpcsTaskResult
	if err := svc.getSwarmingJSON(taskID, ".swarm", &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

func (svc debugSwarmingService) getTaskOutput(c context.Context, taskID string) (string, error) {
	content, err := svc.getContent(taskID, "")
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return "", err
	}
	return string(content), nil
}

// TestableLog is a subclass of Log that interfaces with TestableHandler and
// includes sample test data.
type TestableLog struct{ Log }

// TestableBuild is a subclass of Build that interfaces with TestableHandler and
// includes sample test data.
type TestableBuild struct{ Build }

// TestData returns sample test data.
func (l TestableLog) TestData() []settings.TestBundle {
	return []settings.TestBundle{
		{
			Description: "Basic log",
			Data: templates.Args{
				"Log":    "This is the log",
				"Closed": true,
			},
		},
	}
}

// TestData returns sample test data.
func (b TestableBuild) TestData() []settings.TestBundle {
	basic := resp.MiloBuild{
		Summary: resp.BuildComponent{
			Label:    "Test swarming build",
			Status:   resp.Success,
			Started:  time.Date(2016, 1, 2, 15, 4, 5, 999999999, time.UTC),
			Finished: time.Date(2016, 1, 2, 15, 4, 6, 999999999, time.UTC),
			Duration: time.Second,
		},
	}
	results := []settings.TestBundle{
		{
			Description: "Basic successful build",
			Data:        templates.Args{"Build": basic},
		},
	}
	c := context.Background()
	c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))

	var svc debugSwarmingService
	for _, tc := range getTestCases() {
		build, err := swarmingBuildImpl(c, svc, "foo", tc)
		if err != nil {
			panic(fmt.Errorf("Error while processing %s: %s", tc, err))
		}
		results = append(results, settings.TestBundle{
			Description: tc,
			Data:        templates.Args{"Build": build},
		})
	}
	return results
}
