// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	"github.com/luci/gae/impl/memory"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	memcfg "github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/errors"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/common/model"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/templates"
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

	// References "milo/appengine/build_source/swarming/testdata".
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

// testLogDogClient is a minimal functional LogsClient implementation.
//
// It retains its latest input parameter and returns its configured err (if not
// nil) or resp.
type testLogDogClient struct {
	logdog.LogsClient

	req  interface{}
	resp interface{}
	err  error
}

func (tc *testLogDogClient) Tail(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (
	*logdog.GetResponse, error) {

	tc.req = in
	if tc.err != nil {
		return nil, tc.err
	}
	if tc.resp == nil {
		return nil, coordinator.ErrNoSuchStream
	}
	return tc.resp.(*logdog.GetResponse), nil
}

func logDogClientFunc(tc *testCase) func(context.Context, string) (*coordinator.Client, error) {
	anno := tc.getAnnotation()
	var resp interface{}
	if anno != nil {
		resp = datagramGetResponse("testproject", "foo/bar", tc.getAnnotation())
	}

	return func(c context.Context, host string) (*coordinator.Client, error) {
		return &coordinator.Client{
			Host: host,
			C: &testLogDogClient{
				resp: resp,
			},
		}, nil
	}
}

func datagramGetResponse(project, prefix string, msg proto.Message) *logdog.GetResponse {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return &logdog.GetResponse{
		Project: project,
		Desc: &logpb.LogStreamDescriptor{
			Prefix:      prefix,
			ContentType: miloProto.ContentTypeAnnotations,
			StreamType:  logpb.StreamType_DATAGRAM,
		},
		State: &logdog.LogStreamState{},
		Logs: []*logpb.LogEntry{
			{
				Content: &logpb.LogEntry_Datagram{
					Datagram: &logpb.Datagram{
						Data: data,
					},
				},
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
		svc := debugSwarmingService{tc}
		bl := buildLoader{
			logDogClientFunc: logDogClientFunc(tc),
		}

		build, err := bl.swarmingBuildImpl(c, svc, "foo", tc.name)
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
