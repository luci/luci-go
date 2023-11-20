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
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/googleapi"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/caching/layered"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// bbagentReservedGracePeriod is the time reserved by bbagent in order to have
	// time to have a couple retry rounds for UpdateBuild RPCs
	// TODO(crbug.com/1328646): may need to adjust the grace_period based on
	// UpdateBuild's new performance in Buildbucket Go.
	bbagentReservedGracePeriod = 180

	// runTaskGiveUpTimeoutDefault is the default value for how long to retry
	// the CreateBackendTask before giving up with INFRA_FAILURE.
	runTaskGiveUpTimeoutDefault = 10 * 60 * time.Second

	cipdCacheTTL = 10 * time.Minute
)

type cipdPackageDetails struct {
	Size int64  `json:"size,omitempty"`
	Hash string `json:"hash,omitempty"`
}

type cipdPackageDetailsMap map[string]*cipdPackageDetails

var cipdDescribeBootstrapBundleCache = layered.RegisterCache(layered.Parameters[cipdPackageDetailsMap]{
	ProcessCacheCapacity: 1000,
	GlobalNamespace:      "cipd-describeBootstrapBundle-v1",
	Marshal: func(item cipdPackageDetailsMap) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) (cipdPackageDetailsMap, error) {
		res := cipdPackageDetailsMap{}
		err := json.Unmarshal(blob, &res)
		return res, err
	},
})

type MockCipdClientKey struct{}

func NewCipdClient(ctx context.Context, host string, project string) (client *prpc.Client, err error) {
	if mockClient, ok := ctx.Value(MockCipdClientKey{}).(*prpc.Client); ok {
		return mockClient, nil
	}
	client, err = clients.CreateRawPrpcClient(ctx, host, project)
	return
}

// computeTaskCaches computes the task caches.
func computeTaskCaches(infra *model.BuildInfra) []*pb.CacheEntry {
	caches := make([]*pb.CacheEntry, len(infra.Proto.Backend.GetCaches()))
	for i, c := range infra.Proto.Backend.GetCaches() {
		caches[i] = &pb.CacheEntry{
			EnvVar:           c.GetEnvVar(),
			Name:             c.GetName(),
			Path:             c.GetPath(),
			WaitForWarmCache: c.GetWaitForWarmCache(),
		}
	}
	return caches
}

func computeAgentArgs(build *pb.Build, infra *pb.BuildInfra) (args []string) {
	args = []string{}
	// build-id arg
	args = append(args, "-build-id")
	args = append(args, strconv.FormatInt(build.GetId(), 10))
	// host arg
	args = append(args, "-host")
	args = append(args, infra.Buildbucket.GetHostname())
	// cache-base arg
	args = append(args, "-cache-base")
	args = append(args, infra.Bbagent.GetCacheDir())

	// context-file arg
	args = append(args, "-context-file")
	args = append(args, "${BUILDBUCKET_AGENT_CONTEXT_FILE}")
	return
}

// computeBackendPubsubTopic computes the pubsub topic that should be included
// in RunTaskRequest. Return an empty string if the backend is in lite mode.
func computeBackendPubsubTopic(ctx context.Context, target string, globalCfg *pb.SettingsCfg) (string, error) {
	if globalCfg == nil {
		return "", errors.Reason("error fetching service config").Err()
	}
	for _, backend := range globalCfg.Backends {
		if backend.Target == target {
			switch backend.Mode.(type) {
			case *pb.BackendSetting_LiteMode_:
				return "", nil
			case *pb.BackendSetting_FullMode_:
				return fmt.Sprintf("projects/%s/topics/%s", info.AppID(ctx), backend.GetFullMode().GetPubsubId()), nil
			default:
				return "", errors.Reason("getting pubsub_id from backend %s is not supported", target).Err()
			}
		}
	}
	return "", errors.Reason("backend %s not found in global settings", target).Err()
}

func computeBackendNewTaskReq(ctx context.Context, build *model.Build, infra *model.BuildInfra, requestID string, globalCfg *pb.SettingsCfg) (*pb.RunTaskRequest, error) {
	// Create StartBuildToken and secrets.
	startBuildToken, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_START_BUILD)
	if err != nil {
		return nil, err
	}
	secrets := &pb.BuildSecrets{
		StartBuildToken:               startBuildToken,
		ResultdbInvocationUpdateToken: build.ResultDBUpdateToken,
	}
	backend := infra.Proto.GetBackend()
	if backend == nil {
		return nil, errors.New("infra.Proto.Backend isn't set")
	}
	caches := computeTaskCaches(infra)
	if err != nil {
		return nil, errors.Annotate(err, "RunTaskRequest.Caches could not be created").Err()
	}
	gracePeriod := &durationpb.Duration{
		Seconds: build.Proto.GetGracePeriod().GetSeconds() + bbagentReservedGracePeriod,
	}

	startDeadline := &timestamppb.Timestamp{
		Seconds: build.Proto.GetCreateTime().GetSeconds() + build.Proto.GetSchedulingTimeout().GetSeconds(),
	}

	pubsubTopic, err := computeBackendPubsubTopic(ctx, backend.Task.Id.Target, globalCfg)
	if err != nil {
		return nil, err
	}

	taskReq := &pb.RunTaskRequest{
		BuildbucketHost:  infra.Proto.Buildbucket.Hostname,
		Secrets:          secrets,
		Target:           backend.Task.Id.Target,
		RequestId:        requestID,
		BuildId:          strconv.FormatInt(build.Proto.Id, 10),
		Realm:            build.Realm(),
		BackendConfig:    backend.Config,
		ExecutionTimeout: build.Proto.GetExecutionTimeout(),
		GracePeriod:      gracePeriod,
		Caches:           caches,
		AgentArgs:        computeAgentArgs(build.Proto, infra.Proto),
		Dimensions:       infra.Proto.Backend.GetTaskDimensions(),
		StartDeadline:    startDeadline,
		Experiments:      build.Proto.Input.GetExperiments(),
		PubsubTopic:      pubsubTopic,
	}

	project := build.Proto.Builder.Project
	taskReq.Agent = &pb.RunTaskRequest_AgentExecutable{}
	taskReq.Agent.Source, err = extractCipdDetails(ctx, project, infra.Proto)
	if err != nil {
		return nil, err
	}

	build.Proto.Infra = infra.Proto
	tags := computeTags(ctx, build)
	tagsAny := make([]any, len(tags))
	for i, t := range tags {
		tagsAny[i] = t
	}
	tagsList, err := structpb.NewList(tagsAny)
	if err != nil {
		return nil, err
	}
	if taskReq.BackendConfig == nil {
		taskReq.BackendConfig = &structpb.Struct{}
	}
	taskReq.BackendConfig.Fields["tags"] = structpb.NewListValue(tagsList)
	return taskReq, nil
}

func createCipdDescribeBootstrapBundleRequest(infra *pb.BuildInfra) *cipdpb.DescribeBootstrapBundleRequest {
	prefix := infra.Buildbucket.Agent.Source.GetCipd().GetPackage()
	prefix = strings.TrimSuffix(prefix, "/${platform}")
	return &cipdpb.DescribeBootstrapBundleRequest{
		Prefix:  prefix,
		Version: infra.Buildbucket.Agent.Source.GetCipd().GetVersion(),
	}
}

func computeCipdURL(source *pb.BuildInfra_Buildbucket_Agent_Source, pkg string, details *cipdPackageDetails) (url string) {
	server := source.GetCipd().GetServer()
	version := source.GetCipd().GetVersion()
	return server + "/bootstrap/" + pkg + "/+/" + version
}

// extractCipdDetails returns a map that maps package (Prefix + variant for each variant)
// to a cipdPackageDetails object, which is just the hash and size.
//
// A Cipd client is created and calls DescribeBootstrapBundle to retrieve the data.
func extractCipdDetails(ctx context.Context, project string, infra *pb.BuildInfra) (details map[string]*pb.RunTaskRequest_AgentExecutable_AgentSource, err error) {
	cipdServer := infra.Buildbucket.Agent.Source.GetCipd().GetServer()
	cipdClient, err := NewCipdClient(ctx, cipdServer, project)
	if err != nil {
		return nil, err
	}
	req := createCipdDescribeBootstrapBundleRequest(infra)
	bytes, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	cachePrefix := base64.StdEncoding.EncodeToString(bytes)
	cipdDetails, err := cipdDescribeBootstrapBundleCache.GetOrCreate(ctx, cachePrefix, func() (cipdPackageDetailsMap, time.Duration, error) {
		out := &cipdpb.DescribeBootstrapBundleResponse{}
		err := cipdClient.Call(ctx, "cipd.Repository", "DescribeBootstrapBundle", req, out)
		if err != nil {
			return nil, 0, err
		}
		resp := make(cipdPackageDetailsMap, len(out.Files))
		for _, file := range out.Files {
			resp[file.Package] = &cipdPackageDetails{
				Hash: file.Instance.HexDigest,
				Size: file.Size,
			}
		}
		return resp, cipdCacheTTL, nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "cache error for cipd request").Err()
	}
	details = map[string]*pb.RunTaskRequest_AgentExecutable_AgentSource{}
	for k, v := range cipdDetails {
		val := &pb.RunTaskRequest_AgentExecutable_AgentSource{
			Sha256:    v.Hash,
			SizeBytes: v.Size,
			Url:       computeCipdURL(infra.Buildbucket.Agent.Source, k, v),
		}
		details[k] = val
	}
	return
}

// CreateBackendTask creates a backend task for the build.
func CreateBackendTask(ctx context.Context, buildID int64, requestID string) error {
	entities, err := common.GetBuildEntities(ctx, buildID, model.BuildKind, model.BuildInfraKind)
	if err != nil {
		return errors.Annotate(err, "failed to get build %d", buildID).Err()
	}
	bld := entities[0].(*model.Build)
	infra := entities[1].(*model.BuildInfra)

	if infra.Proto.GetBackend().GetTask().GetId().GetId() != "" {
		// This task is likely a retry.
		// It could happen if the previous RunTask attempt(s) failed, but a backend
		// task was actually created and associated with the build in the backup
		// flow.
		// Bail out.
		logging.Infof(ctx, "build %d has associated with task %q", buildID, infra.Proto.Backend.Task.Id)
		return nil
	}

	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return errors.Annotate(err, "could not get global settings config").Err()
	}

	var backendCfg *pb.BackendSetting
	for _, backend := range globalCfg.GetBackends() {
		if backend.Target == infra.Proto.Backend.Task.Id.Target {
			backendCfg = backend
		}
	}
	if backendCfg == nil {
		return tq.Fatal.Apply(errors.Reason("failed to get backend config from global settings").Err())
	}

	var runTaskGiveUpTimeout time.Duration
	if backendCfg.TaskCreatingTimeout.GetSeconds() == 0 {
		runTaskGiveUpTimeout = runTaskGiveUpTimeoutDefault
	} else {
		runTaskGiveUpTimeout = backendCfg.TaskCreatingTimeout.AsDuration()
	}

	// If task creation has already expired, fail the build immediately.
	if clock.Now(ctx).Sub(bld.CreateTime) >= runTaskGiveUpTimeout {
		dsPutErr := failBuild(ctx, buildID, "Backend task creation failure.")
		if dsPutErr != nil {
			return dsPutErr
		}
		return tq.Fatal.Apply(errors.Reason("creating backend task for build %d has expired after %s", buildID, runTaskGiveUpTimeout.String()).Err())
	}

	// Create a backend task client
	backend, err := clients.NewBackendClient(ctx, bld.Proto.Builder.Project, infra.Proto.Backend.Task.Id.Target, globalCfg)
	if err != nil {
		return tq.Fatal.Apply(errors.Annotate(err, "failed to connect to backend service").Err())
	}

	taskReq, err := computeBackendNewTaskReq(ctx, bld, infra, requestID, globalCfg)
	if err != nil {
		return tq.Fatal.Apply(err)
	}

	// Create a backend task via RunTask
	taskResp, err := backend.RunTask(ctx, taskReq)
	now := clock.Now(ctx)
	if err != nil {
		// Give up if HTTP 500s are happening continuously. Otherwise re-throw the
		// error so Cloud Tasks retries the task.
		if apiErr, _ := err.(*googleapi.Error); apiErr == nil || apiErr.Code >= 500 {
			if now.Sub(bld.CreateTime) < runTaskGiveUpTimeout {
				return transient.Tag.Apply(errors.Annotate(err, "failed to create a backend task").Err())
			}
			logging.Errorf(ctx, "Give up backend task creation retry after %s", runTaskGiveUpTimeout.String())
		}
		logging.Errorf(ctx, "Backend task creation failure:%s. RunTask request: %+v", err, taskReq)
		dsPutErr := failBuild(ctx, bld.ID, "Backend task creation failure.")
		if dsPutErr != nil {
			return dsPutErr
		}
		return tq.Fatal.Apply(errors.Annotate(err, "failed to create a backend task").Err())
	}
	if taskResp.Task.GetUpdateId() == 0 {
		return tq.Fatal.Apply(errors.Reason("task returned with an updateID of 0").Err())
	}
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entities, err := common.GetBuildEntities(ctx, buildID, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			return errors.Annotate(err, "failed to get build %d", buildID).Err()
		}
		bld = entities[0].(*model.Build)
		infra = entities[1].(*model.BuildInfra)

		infra.Proto.Backend.Task = taskResp.Task

		// Update Build entity.
		bld.Proto.UpdateTime = timestamppb.New(now)
		target := taskResp.Task.Id.Target
		for _, backendSetting := range globalCfg.Backends {
			if backendSetting.Target == target {
				if backendSetting.GetFullMode().GetBuildSyncSetting() != nil {
					bld.BackendTarget = target
					interval := backendSetting.GetFullMode().GetBuildSyncSetting().GetSyncIntervalSeconds()
					if interval > 0 {
						bld.BackendSyncInterval = time.Duration(interval) * time.Second
					}
					bld.GenerateNextBackendSyncTime(ctx, backendSetting.GetFullMode().GetBuildSyncSetting().GetShards())
				}
				break
			}
		}

		return datastore.Put(ctx, bld, infra)
	}, nil)
	if txErr != nil {
		logging.Errorf(ctx, "Task failed to save in BuildInfra: %s", taskResp.String())
		return transient.Tag.Apply(errors.Annotate(err, "failed to save the backend task in BuildInfra").Err())
	}
	return nil
}
