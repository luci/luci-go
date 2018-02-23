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

package testdata

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
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	miloProto "go.chromium.org/luci/common/proto/milo"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1/fakelogs"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/templates"
)

type TestCase struct {
	Name string

	SwarmingRelDir string

	SwarmResult string
	TaskOutput  string
	Annotations string
}

func GetTestCases(swarmingRelDir string) []*TestCase {
	testCases := make(map[string]*TestCase)

	// References "milo/buildsource/swarming/testdata".
	testdata := filepath.Join(swarmingRelDir, "testdata")
	f, err := ioutil.ReadDir(testdata)
	if err != nil {
		panic(err)
	}
	for _, fi := range f {
		fileName := fi.Name()
		parts := strings.SplitN(fileName, ".", 2)

		if len(parts) > 1 && parts[1] == "go" {
			continue
		}

		name := parts[0]

		tc := testCases[name]
		if tc == nil {
			tc = &TestCase{Name: name}
			testCases[name] = tc
		}
		tc.SwarmingRelDir = swarmingRelDir

		switch {
		case len(parts) == 1:
			tc.TaskOutput = fileName
		case parts[1] == "swarm":
			tc.SwarmResult = fileName
		case parts[1] == "pb.txt":
			tc.Annotations = fileName
		}
	}

	// Order test cases by name.
	names := make([]string, 0, len(testCases))
	for name := range testCases {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]*TestCase, len(names))
	for i, name := range names {
		results[i] = testCases[name]
	}

	return results
}

func (tc *TestCase) GetHost() string { return "example.com" }

func (tc *TestCase) GetSwarmingRequest(context.Context, string) (*swarming.SwarmingRpcsTaskRequest, error) {
	return nil, errors.New("not implemented")
}

func (tc *TestCase) GetSwarmingResult(context.Context, string) (*swarming.SwarmingRpcsTaskResult, error) {
	var sr swarming.SwarmingRpcsTaskResult
	data := tc.getContent(tc.SwarmResult)
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

func (tc *TestCase) GetTaskOutput(context.Context, string) (string, error) {
	return string(tc.getContent(tc.TaskOutput)), nil
}

func (tc *TestCase) InjectLogdogClient(c context.Context) context.Context {
	fCl := fakelogs.NewClient()
	c = rawpresentation.InjectFakeLogdogClient(c, fCl)
	if tc.Annotations == "" {
		return c
	}

	rslt, err := tc.GetSwarmingResult(nil, "")
	if err != nil {
		panic(err)
	}

	url, err := types.ParseURL(strpair.ParseMap(rslt.Tags).Get("log_location"))
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

func (tc *TestCase) getContent(name string) []byte {
	if name == "" {
		return nil
	}

	// ../swarming below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	path := filepath.Join(tc.SwarmingRelDir, "testdata", name)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("failed to read [%s]: %s", path, err))
	}
	return data
}

func (tc *TestCase) getAnnotation() *miloProto.Step {
	var step miloProto.Step
	data := tc.getContent(tc.Annotations)
	if len(data) == 0 {
		return nil
	}

	if err := proto.UnmarshalText(string(data), &step); err != nil {
		panic(fmt.Errorf("failed to unmarshal text protobuf [%s]: %s", tc.Annotations, err))
	}
	return &step
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

// SwarmingService is an interface that fetches data from Swarming.
//
// In production, this is fetched from a Swarming host. For testing, this can
// be replaced with a mock.
type SwarmingService interface {
	GetHost() string
	GetSwarmingResult(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error)
	GetSwarmingRequest(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskRequest, error)
	GetTaskOutput(c context.Context, taskID string) (string, error)
}

type SwarmingBuildImplFn func(c context.Context, svc SwarmingService, taskID string) (*ui.MiloBuild, error)

// BuildTestData returns sample test data for swarming build pages.
func BuildTestData(swarmingRelDir string, swarmingBuildImpl SwarmingBuildImplFn) []common.TestBundle {
	basic := ui.MiloBuild{
		Summary: ui.BuildComponent{
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
	c = testconfig.WithCommonClient(c, memcfg.New(AclConfigs))
	c = auth.WithState(c, &authtest.FakeState{
		Identity:       "user:alicebob@google.com",
		IdentityGroups: []string{"all", "googlers"},
	})
	c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))

	for _, tc := range GetTestCases(swarmingRelDir) {
		c := tc.InjectLogdogClient(c)

		build, err := swarmingBuildImpl(c, tc, tc.Name)
		if err != nil {
			panic(fmt.Errorf("Error while processing %s: %s", tc.Name, err))
		}
		results = append(results, common.TestBundle{
			Description: tc.Name,
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

var AclConfigs = map[string]memcfg.ConfigSet{
	"projects/secret": {
		"project.cfg": secretProjectCfg,
	},
	"projects/opensource": {
		"project.cfg": publicProjectCfg,
	},
}
