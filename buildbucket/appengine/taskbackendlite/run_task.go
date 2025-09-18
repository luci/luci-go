// Copyright 2023 The LUCI Authors.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	DefaultTaskCreationTimeout = 10 * time.Minute
	TopicIDFormat              = "taskbackendlite-%s"
)

// TaskBackendLite implements TaskBackendLiteServer.
type TaskBackendLite struct {
	grpcpb.UnimplementedTaskBackendLiteServer
}

// TaskNotification is to notify users about a task creation event.
// TaskBackendLite will publish this message to Cloud pubsub in its json format.
type TaskNotification struct {
	BuildID         string `json:"build_id"`
	StartBuildToken string `json:"start_build_token"`
}

// RunTask handles the request to create a task. Implements pb.TaskBackendLiteServer.
func (t *TaskBackendLite) RunTask(ctx context.Context, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	logging.Debugf(ctx, "%q called RunTask with request %s", auth.CurrentIdentity(ctx), proto.MarshalTextString(req))
	project, err := t.checkPerm(ctx)
	if err != nil {
		return nil, err
	}

	// dummyTaskID format is <BuildId>_<RequestId> to make it globally unique.
	dummyTaskID := fmt.Sprintf("%s_%s", req.BuildId, req.RequestId)
	resp := &pb.RunTaskResponse{
		Task: &pb.Task{
			Id: &pb.TaskID{
				Id:     dummyTaskID,
				Target: req.Target,
			},
			UpdateId: 1,
		},
	}

	// Not process the same request more than once to make the entire operation idempotent.
	cache := caching.GlobalCache(ctx, "taskbackendlite-run-task")
	if cache == nil {
		return nil, status.Errorf(codes.Internal, "cannot find the global cache")
	}
	taskCached, err := cache.Get(ctx, dummyTaskID)
	switch {
	case errors.Is(err, caching.ErrCacheMiss):
	case err != nil:
		return nil, status.Errorf(codes.Internal, "cannot read %s from the global cache", dummyTaskID)
	case taskCached != nil:
		logging.Infof(ctx, "this task(%s) has been handled before, ignoring", dummyTaskID)
		return resp, nil
	}

	// Publish the msg into Pubsub by using the project-scoped identity.
	psClient, err := clients.NewPubsubClient(ctx, info.AppID(ctx), project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error when creating Pub/Sub client: %s", err)
	}
	defer psClient.Close()
	topicID := fmt.Sprintf(TopicIDFormat, project)
	topic := psClient.Topic(topicID)
	defer topic.Stop()
	data, err := json.MarshalIndent(&TaskNotification{
		BuildID:         req.BuildId,
		StartBuildToken: req.Secrets.StartBuildToken,
	}, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to compose pubsub message: %s", err)
	}

	proj, bucket, builder := extractBuilderInfo(req)
	result := topic.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"dummy_task_id": dummyTaskID, // can be used for deduplication on the subscriber side.
			"project":       proj,
			"bucket":        bucket,
			"builder":       builder,
		},
	})
	if _, err = result.Get(ctx); err != nil {
		switch status.Code(err) {
		case codes.PermissionDenied:
			return nil, status.Errorf(codes.PermissionDenied, "luci project scoped account(%s) does not have the permission to publish to topic %s", project, topicID)
		case codes.NotFound:
			return nil, status.Errorf(codes.InvalidArgument, "topic %s does not exist on Cloud project %s", topicID, psClient.Project())
		}
		return nil, status.Errorf(codes.Internal, "failed to publish pubsub message: %s", err)
	}

	// Save into cache
	if err := cache.Set(ctx, dummyTaskID, []byte{1}, DefaultTaskCreationTimeout); err != nil {
		// Ignore it. The cache is to dedup Buildbucket requests. Duplicate
		// requests rarely happen. But if it returns the error back, Buildbucket
		// will send the same request again, which shoots ourselves in the foot.
		logging.Warningf(ctx, "failed to save the task %s into cache", dummyTaskID)
	}
	return resp, nil
}

// checkPerm checks if the caller has the correct access.
// Returns PermissionDenied if the it has no permission.
func (*TaskBackendLite) checkPerm(ctx context.Context) (string, error) {
	s := auth.GetState(ctx)
	if s == nil {
		return "", status.Errorf(codes.Internal, "the auth state is not properly configured")
	}
	switch peer := s.PeerIdentity(); {
	case strings.HasSuffix(info.AppID(ctx), "-dev") && peer == identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"): // on Dev
	case !strings.HasSuffix(info.AppID(ctx), "-dev") && peer == identity.Identity("user:cr-buildbucket@appspot.gserviceaccount.com"): // on Prod
	default:
		return "", status.Errorf(codes.PermissionDenied, "the peer %q is not allowed to access this task backend", peer)
	}

	// In TaskBackendLite protocal, Buildbucket uses Project-scoped identity:
	// https://chromium.googlesource.com/infra/luci/luci-go/+/574d2290aefbfe586b31bb43f89d9b2027f73a70/buildbucket/proto/backend.proto#337
	user := s.User().Identity
	if user.Kind() != identity.Project {
		return "", status.Errorf(codes.PermissionDenied, "The caller's user identity %q is not a project identity", user)
	}
	return user.Value(), nil
}

// extractBuilderInfo extracts this RunTaskRequest's project, bucket and
// builder info. If any info doesn't appear, return the empty string.
func extractBuilderInfo(req *pb.RunTaskRequest) (string, string, string) {
	proj, bucket, builder := "", "", ""
	for _, tag := range req.BackendConfig.GetFields()["tags"].GetListValue().GetValues() {
		switch key, val, _ := strings.Cut(tag.GetStringValue(), ":"); {
		case key == "builder":
			builder = val
		case key == "buildbucket_bucket":
			proj, bucket, _ = protoutil.ParseBucketID(val)
		}
	}
	return proj, bucket, builder
}
