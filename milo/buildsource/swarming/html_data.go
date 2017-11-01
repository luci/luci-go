// Copyright 2015 The LUCI Authors.
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

package swarming

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/gae/impl/memory"
	"golang.org/x/net/context"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1/fakelogs"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/templates"
)

type testCase struct {
	name string

	swarmingRelDir string

	swarmResult string
	swarmOutput string
	annotations string
}

func getTestCases(swarmingRelDir string) []*testCase {
	testCases := make(map[string]*testCase)

	// References "milo/buildsource/swarming/testdata".
	testdata := filepath.Join(swarmingRelDir, "testdata")
	f, err := ioutil.ReadDir(testdata)
	if err != nil {
		panic(err)
	}
	for _, fi := range f {
		fileName := fi.Name()
		parts := strings.SplitN(fileName, ".", 2)

		name := parts[0]
		tc := testCases[name]
		if tc == nil {
			tc = &testCase{name: name}
			testCases[name] = tc
		}
		tc.swarmingRelDir = swarmingRelDir

		switch {
		case len(parts) == 1:
			tc.swarmOutput = fileName
		case parts[1] == "swarm":
			tc.swarmResult = fileName
		case parts[1] == "pb.txt":
			tc.annotations = fileName
		}
	}

	// Order test cases by name.
	names := make([]string, 0, len(testCases))
	for name := range testCases {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]*testCase, len(names))
	for i, name := range names {
		results[i] = testCases[name]
	}

	return results
}

func (tc *testCase) injectLogdogClient(c context.Context) context.Context {
	fCl := fakelogs.NewClient()
	c = rawpresentation.InjectFakeLogdogClient(c, fCl)
	if tc.annotations == "" {
		return c
	}

	url, err := types.ParseURL(strpair.ParseMap(tc.getSwarmingResult().Tags).Get("log_location"))
	if err != nil {
		panic(err)
	}
	prefix, path := url.Path.Split()

	s, err := fCl.OpenDatagramStream(prefix, path, &streamproto.Flags{
		ContentType: miloProto.ContentTypeAnnotations,
	})
	if err != nil {
		panic(err)
	}
	data, err := proto.Marshal(tc.getAnnotation())
	if err != nil {
		panic(err)
	}
	if _, err := s.Write(data); err != nil {
		panic(err)
	}
	if err := s.Close(); err != nil {
		panic(err)
	}
	return c
}

func (tc *testCase) getContent(name string) []byte {
	if name == "" {
		return nil
	}

	// ../swarming below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	path := filepath.Join(tc.swarmingRelDir, "testdata", name)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("failed to read [%s]: %s", path, err))
	}
	return data
}

func (tc *testCase) getSwarmingResult() *swarming.SwarmingRpcsTaskResult {
	var sr swarming.SwarmingRpcsTaskResult
	data := tc.getContent(tc.swarmResult)
	if err := json.Unmarshal(data, &sr); err != nil {
		panic(fmt.Errorf("failed to unmarshal [%s]: %s", tc.swarmResult, err))
	}
	return &sr
}

func (tc *testCase) getSwarmingOutput() string {
	return string(tc.getContent(tc.swarmOutput))
}

func (tc *testCase) getAnnotation() *miloProto.Step {
	var step miloProto.Step
	data := tc.getContent(tc.annotations)
	if len(data) == 0 {
		return nil
	}

	if err := proto.UnmarshalText(string(data), &step); err != nil {
		panic(fmt.Errorf("failed to unmarshal text protobuf [%s]: %s", tc.annotations, err))
	}
	return &step
}

type debugSwarmingService struct {
	tc *testCase
}

func (svc debugSwarmingService) getHost() string { return "example.com" }

func (svc debugSwarmingService) getSwarmingResult(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskResult, error) {

	return svc.tc.getSwarmingResult(), nil
}

func (svc debugSwarmingService) getSwarmingRequest(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskRequest, error) {

	return nil, errors.New("not implemented")
}

func (svc debugSwarmingService) getTaskOutput(c context.Context, taskID string) (string, error) {
	return svc.tc.getSwarmingOutput(), nil
}

// LogTestData returns sample test data for log pages.
func LogTestData() []common.TestBundle {
	return []common.TestBundle{
		{
			Description: "Basic log",
			Data: templates.Args{
				"Log":    "This is the log",
				"Closed": true,
			},
		},
	}
}

// BuildTestData returns sample test data for swarming build pages.
func BuildTestData(swarmingRelDir string) []common.TestBundle {
	basic := resp.MiloBuild{
		Summary: resp.BuildComponent{
			Label:    "Test swarming build",
			Status:   model.Success,
			Started:  time.Date(2016, 1, 2, 15, 4, 5, 999999999, time.UTC),
			Finished: time.Date(2016, 1, 2, 15, 4, 6, 999999999, time.UTC),
			Duration: time.Second,
		},
	}
	results := []common.TestBundle{
		{
			Description: "Basic successful build",
			Data:        templates.Args{"Build": basic},
		},
	}
	c := context.Background()
	c = memory.UseWithAppID(c, "dev~luci-milo")
	c = testconfig.WithCommonClient(c, memcfg.New(aclConfgs))
	c = auth.WithState(c, &authtest.FakeState{
		Identity:       "user:alicebob@google.com",
		IdentityGroups: []string{"all", "googlers"},
	})
	c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))

	for _, tc := range getTestCases(swarmingRelDir) {
		c := tc.injectLogdogClient(c)
		svc := debugSwarmingService{tc}

		build, err := SwarmingBuildImpl(c, svc, tc.name)
		if err != nil {
			panic(fmt.Errorf("Error while processing %s: %s", tc.name, err))
		}
		results = append(results, common.TestBundle{
			Description: tc.name,
			Data:        templates.Args{"Build": build},
		})
	}
	return results
}

var secretProjectCfg = `
name: "secret"
access: "group:googlers"
`

var publicProjectCfg = `
name: "opensource"
access: "group:all"
`

var aclConfgs = map[string]memcfg.ConfigSet{
	"projects/secret": {
		"project.cfg": secretProjectCfg,
	},
	"projects/opensource": {
		"project.cfg": publicProjectCfg,
	},
}
