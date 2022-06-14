// Copyright 2022 The LUCI Authors.
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

package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/tink/go/subtle/random"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// bbagentReservedGracePeriod is the time reserved by bbagent in order to have
	// time to have a couple retry rounds for UpdateBuild RPCs
	// TODO(crbug.com/1328646): may need to adjust the grace_period based on
	// UpdateBuild's new performance in Buildbucket Go.
	bbagentReservedGracePeriod = 180

	// cacheDir is the path, relative to the swarming run dir, to the directory that
	// contains the mounted swarming named caches. It will be prepended to paths of
	// caches defined in global or builder configs.
	cacheDir = "cache"

	// pubsubTopicTemplate is the topic template where Swarming publishes
	// notifications on the task update.
	pubsubTopicTemplate = "projects/%s/topics/swarming"

	// pubSubUserDataTemplate is the Swarming topic user data template.
	pubSubUserDataTemplate = `{
		"build_id": %d,
		"created_ts": %d,
		"swarming_hostname": %s
}`
)

func createSwarmingTask(ctx context.Context, build *model.Build, swarm clients.SwarmingClient) error {
	taskReq, err := computeSwarmingNewTaskReq(ctx, build)
	if err != nil {
		return err
	}

	// Insert secret bytes.
	token, err := generateBuildToken(build.ID)
	if err != nil {
		return err
	}
	secrets := &pb.BuildSecrets{
		BuildToken:                    token,
		ResultdbInvocationUpdateToken: build.ResultDBUpdateToken,
	}
	secretsBytes, err := proto.Marshal(secrets)
	if err != nil {
		return err
	}
	for _, t := range taskReq.TaskSlices {
		t.Properties.SecretBytes = base64.RawURLEncoding.EncodeToString(secretsBytes)
	}

	// TODO(crbug.com/1328646): call swarming and update Build entity with token
	// and new task id.
	return nil
}

func computeSwarmingNewTaskReq(ctx context.Context, build *model.Build) (*swarming.SwarmingRpcsNewTaskRequest, error) {
	sw := build.Proto.GetInfra().GetSwarming()
	if sw == nil {
		return nil, errors.New("build.Proto.Infra.Swarming isn't set")
	}
	taskReq := &swarming.SwarmingRpcsNewTaskRequest{
		// to prevent accidental multiple task creation
		RequestUuid: strconv.FormatInt(build.ID, 10),
		Name:        fmt.Sprintf("bb-%d-%s", build.ID, build.BuilderID),
		Realm:       build.Realm(),
		Tags:        computeTags(ctx, build),
		Priority:    int64(sw.Priority),
	}

	if build.Proto.Number > 0 {
		taskReq.Name = fmt.Sprintf("%s-%d", taskReq.Name, build.Proto.Number)
	}

	taskSlices, err := computeTaskSlice(build)
	if err != nil {
		errors.Annotate(err, "failed to computing task slices").Err()
	}
	taskReq.TaskSlices = taskSlices

	// Only makes swarming to track the build's parent if Buildbucket doesn't
	// track.
	// Buildbucket should track the parent/child build relationships for all
	// Buildbucket Builds.
	// Except for children of led builds, whose parents are still tracked by
	// swarming using sw.parent_run_id.
	// TODO(crbug.com/1031205): remove the check on
	// luci.buildbucket.parent_tracking after this experiment is on globally and
	// we're ready to remove it.
	if sw.ParentRunId != "" && (len(build.Proto.AncestorIds) == 0 ||
		strings.Contains(build.ExperimentsString(), buildbucket.ExperimentParentTracking)) {
		taskReq.ParentTaskId = sw.ParentRunId
	}

	if sw.TaskServiceAccount != "" {
		taskReq.ServiceAccount = sw.TaskServiceAccount
	}

	taskReq.PubsubTopic = fmt.Sprintf(pubsubTopicTemplate, info.AppID(ctx))
	taskReq.PubsubUserdata = fmt.Sprintf(pubSubUserDataTemplate, build.ID, clock.Now(ctx).UnixNano()/1000, sw.Hostname)

	return taskReq, err
}

// computeTags computes the Swarming task request tags to use.
// Note it doesn't compute kitchen related tags.
func computeTags(ctx context.Context, build *model.Build) []string {
	tags := []string{
		"buildbucket_bucket:" + build.BucketID,
		fmt.Sprintf("buildbucket_build_id:%d", build.ID),
		fmt.Sprintf("buildbucket_hostname:%s.appspot.com", info.AppID(ctx)),
		"luci_project:" + build.Project,
	}
	if build.Canary {
		tags = append(tags, "buildbucket_template_canary:1")
	} else {
		tags = append(tags, "buildbucket_template_canary:0")
	}

	tags = append(tags, build.Tags...)
	sort.Strings(tags)
	return tags
}

// generateBuildToken generates base64 encoded byte string token.
// In the future, it will be replaced by a self-verifiable token.
func generateBuildToken(buildID int64) (string, error) {
	tkBody := &pb.TokenBody{
		BuildId: buildID,
		Purpose: pb.TokenBody_BUILD,
		State:   random.GetRandomBytes(16),
	}

	tkBytes, err := proto.Marshal(tkBody)
	if err != nil {
		return "", err
	}
	tkEnvelop := &pb.TokenEnvelope{
		Version: pb.TokenEnvelope_UNENCRYPTED_PASSWORD_LIKE,
		Payload: tkBytes,
	}
	tkeBytes, err := proto.Marshal(tkEnvelop)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tkeBytes)
	return token, nil
}

// computeTaskSlice computes swarming task slices.
// build.Proto.Infra must be set.
func computeTaskSlice(build *model.Build) ([]*swarming.SwarmingRpcsTaskSlice, error) {
	// expiration_secs -> []*SwarmingRpcsStringPair
	dims := map[int64][]*swarming.SwarmingRpcsStringPair{}
	for _, cache := range build.Proto.GetInfra().GetSwarming().GetCaches() {
		expSecs := cache.WaitForWarmCache.GetSeconds()
		if _, ok := dims[expSecs]; !ok {
			dims[expSecs] = []*swarming.SwarmingRpcsStringPair{}
		}
		dims[expSecs] = append(dims[expSecs], &swarming.SwarmingRpcsStringPair{
			Key:   "caches",
			Value: cache.Name,
		})
	}
	for _, dim := range build.Proto.GetInfra().GetSwarming().GetTaskDimensions() {
		expSecs := dim.Expiration.GetSeconds()
		if _, ok := dims[expSecs]; !ok {
			dims[expSecs] = []*swarming.SwarmingRpcsStringPair{}
		}
		dims[expSecs] = append(dims[expSecs], &swarming.SwarmingRpcsStringPair{
			Key:   dim.Key,
			Value: dim.Value,
		})
	}

	// extract base dim and delete it from the map.
	baseDim, ok := dims[0]
	if !ok {
		baseDim = []*swarming.SwarmingRpcsStringPair{}
	}
	delete(dims, 0)
	if len(dims) > 6 {
		return nil, errors.New("At most 6 different expiration_secs to be allowed in swarming")
	}

	baseSlice := &swarming.SwarmingRpcsTaskSlice{
		ExpirationSecs:  build.Proto.GetSchedulingTimeout().GetSeconds(),
		WaitForCapacity: build.Proto.GetWaitForCapacity(),
		Properties: &swarming.SwarmingRpcsTaskProperties{
			CipdInput:            computeCipdInput(build),
			ExecutionTimeoutSecs: build.Proto.GetExecutionTimeout().GetSeconds(),
			GracePeriodSecs:      build.Proto.GetGracePeriod().GetSeconds() + bbagentReservedGracePeriod,
			Caches:               computeTaskSliceCaches(build),
			Dimensions:           baseDim,
			EnvPrefixes:          computeEnvPrefixes(build),
			Env: []*swarming.SwarmingRpcsStringPair{
				{Key: "BUILDBUCKET_EXPERIMENTAL", Value: strings.ToUpper(strconv.FormatBool(build.Experimental))},
			},
			Command: computeCommand(build),
		},
	}

	// sort dims map by expiration_sec.
	var expSecs []int
	for expSec := range dims {
		expSecs = append(expSecs, int(expSec))
	}
	sort.Ints(expSecs)

	// Create extra task slices by copying the base task slice. Adding the
	// corresponding expiration and desired dimensions
	lastExp := 0
	taskSlices := make([]*swarming.SwarmingRpcsTaskSlice, len(expSecs)+1)
	for i, sec := range expSecs {
		prop := &swarming.SwarmingRpcsTaskProperties{}
		if err := deepCopy(baseSlice.Properties, prop); err != nil {
			return nil, err
		}
		taskSlices[i] = &swarming.SwarmingRpcsTaskSlice{
			ExpirationSecs: int64(sec - lastExp),
			Properties:     prop,
		}
		// dims[i] should be added into all previous non-expired task slices.
		for j := 0; j <= i; j++ {
			taskSlices[j].Properties.Dimensions = append(taskSlices[j].Properties.Dimensions, dims[int64(sec)]...)
		}
		lastExp = sec
	}

	// Tweak expiration on the baseSlice, which is the last slice.
	exp := max(int(baseSlice.ExpirationSecs)-lastExp, 60)
	baseSlice.ExpirationSecs = int64(exp)
	taskSlices[len(taskSlices)-1] = baseSlice

	sortDim := func(strPairs []*swarming.SwarmingRpcsStringPair) {
		sort.Slice(strPairs, func(i, j int) bool {
			if strPairs[i].Key == strPairs[j].Key {
				return strPairs[i].Value < strPairs[j].Value
			}
			return strPairs[i].Key < strPairs[j].Key
		})
	}
	// sort dimensions in each task slice.
	for _, t := range taskSlices {
		sortDim(t.Properties.Dimensions)
	}
	return taskSlices, nil
}

// computeTaskSliceCaches computes the task slice caches.
func computeTaskSliceCaches(build *model.Build) []*swarming.SwarmingRpcsCacheEntry {
	caches := make([]*swarming.SwarmingRpcsCacheEntry, len(build.Proto.Infra.Swarming.GetCaches()))
	for i, c := range build.Proto.Infra.Swarming.GetCaches() {
		caches[i] = &swarming.SwarmingRpcsCacheEntry{
			Name: c.Name,
			Path: filepath.Join(cacheDir, c.Path),
		}
	}
	return caches
}

// computeCipdInput returns swarming task CIPD input.
// Note: this function only considers v2 bbagent builds.
// The build.Proto.Infra.Buildbucket.Agent.Source must be set
func computeCipdInput(build *model.Build) *swarming.SwarmingRpcsCipdInput {
	return &swarming.SwarmingRpcsCipdInput{
		Packages: []*swarming.SwarmingRpcsCipdPackage{{
			PackageName: build.Proto.GetInfra().GetBuildbucket().GetAgent().GetSource().GetCipd().GetPackage(),
			Version:     build.Proto.GetInfra().GetBuildbucket().GetAgent().GetSource().GetCipd().GetVersion(),
			Path:        ".",
		}},
	}
}

// computeEnvPrefixes returns env_prefixes key in swarming properties.
// Note: this function only considers v2 bbagent builds.
func computeEnvPrefixes(build *model.Build) []*swarming.SwarmingRpcsStringListPair {
	prefixesMap := map[string][]string{}
	for _, c := range build.Proto.GetInfra().GetSwarming().GetCaches() {
		if c.EnvVar != "" {
			if _, ok := prefixesMap[c.EnvVar]; !ok {
				prefixesMap[c.EnvVar] = []string{}
			}
			prefixesMap[c.EnvVar] = append(prefixesMap[c.EnvVar], filepath.Join(cacheDir, c.Path))
		}
	}
	var keys []string
	for key := range prefixesMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	prefixes := make([]*swarming.SwarmingRpcsStringListPair, len(keys))
	for i, key := range keys {
		prefixes[i] = &swarming.SwarmingRpcsStringListPair{
			Key:   key,
			Value: prefixesMap[key],
		}
	}
	return prefixes
}

// computeCommand computes the command for bbagent.
func computeCommand(build *model.Build) []string {
	bbagentGetBuildEnabled := false
	for _, exp := range build.Experiments {
		if exp == buildbucket.ExperimentBBAgentGetBuild {
			bbagentGetBuildEnabled = true
			break
		}
	}

	if bbagentGetBuildEnabled {
		return []string{
			"bbagent${EXECUTABLE_SUFFIX}",
			"-host",
			build.Proto.GetInfra().GetBuildbucket().GetHostname(),
			"-build-id",
			strconv.FormatInt(build.ID, 10),
		}
	}

	return []string{
		"bbagent${EXECUTABLE_SUFFIX}",
		bbinput.Encode(&pb.BBAgentArgs{
			Build:                  build.Proto,
			CacheDir:               build.Proto.GetInfra().GetBbagent().GetCacheDir(),
			KnownPublicGerritHosts: build.Proto.GetInfra().GetBuildbucket().GetKnownPublicGerritHosts(),
			PayloadPath:            build.Proto.GetInfra().GetBbagent().GetPayloadPath(),
		}),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// deepCopy deep copies src to dst using json marshaling for non-proto messages.
func deepCopy(src, dst interface{}) error {
	srcBytes, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(srcBytes, dst)
}
