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
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/api/googleapi"

	grpc "google.golang.org/grpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// runTaskGiveUpTimeout indicates how long to retry
	// the CreateBackendTask before giving up with INFRA_FAILURE.
	runTaskGiveUpTimeout = 10 * 60 * time.Second
)

type MockTaskBackendClientKey struct{}

type TaskBackendClient interface {
	RunTask(ctx context.Context, taskReq *pb.RunTaskRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

func newTaskBackendClient(ctx context.Context, host string, project string) (TaskBackendClient, error) {
	if mockClient, ok := ctx.Value(MockTaskBackendClientKey{}).(TaskBackendClient); ok {
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}

	return pb.NewTaskBackendPRPCClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    host,
		Options: prpc.DefaultOptions(),
	}), nil
}

// BackendClient is the client to communicate with TaskBackend.
// It wraps a pb.TaskBackendClient.
type BackendClient struct {
	client TaskBackendClient
}

// NewBackendClient creates a client to communicate with Buildbucket.
func NewBackendClient(ctx context.Context, bld *pb.Build) (*BackendClient, error) {
	hostnname, err := computeHostnameFromTarget(ctx, bld.Infra.Backend.Task.Id.Target)
	if err != nil {
		return nil, err
	}

	client, err := newTaskBackendClient(ctx, hostnname, bld.Builder.Project)
	if err != nil {
		return nil, err
	}

	return &BackendClient{
		client: client,
	}, nil
}

// RunTask returns for the requested task.
func (c *BackendClient) RunTask(ctx context.Context, taskReq *pb.RunTaskRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return c.client.RunTask(ctx, taskReq)
}

func computeHostnameFromTarget(ctx context.Context, target string) (hostname string, err error) {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return "", errors.Annotate(err, "could not get global settings config").Err()
	}
	for _, config := range globalCfg.Backends {
		if config.Target == target {
			return config.Hostname, nil
		}
	}
	return "", errors.Reason("could not find target in global config settings").Err()
}

// RunTaskRequest related code
func computeRequestID(hostname string) uuid.UUID {
	// RequestId = current timestamp + GAE Hostname (which is random)
	timpestamp := time.Now().Unix()
	inputStr := strconv.FormatInt(timpestamp, 10) + hostname
	id := uuid.NewSHA1(uuid.Nil, []byte(inputStr))
	return id
}

func computeBackendNewTaskReq(ctx context.Context, build *model.Build, infra *model.BuildInfra) (*pb.RunTaskRequest, error) {
	backend := infra.Proto.GetBackend()
	if backend == nil {
		return nil, errors.New("infra.Proto.Backend isn't set")
	}

	reqID := computeRequestID(infra.Proto.Buildbucket.Hostname)

	taskReq := &pb.RunTaskRequest{
		Target:        backend.Task.Id.Target,
		RequestId:     reqID.String(),
		BuildId:       strconv.FormatInt(build.Proto.Id, 10),
		Realm:         build.Realm(),
		BackendConfig: backend.Config,
	}
	return taskReq, nil
}

// CreateBackendTask creates a backend task for the build.
func CreateBackendTask(ctx context.Context, buildID int64) error {
	bld := &model.Build{ID: buildID}
	infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
	switch err := datastore.Get(ctx, bld, infra); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		return tq.Fatal.Apply(errors.Annotate(err, "build %d or buildInfra not found", buildID).Err())
	case err != nil:
		return transient.Tag.Apply(errors.Annotate(err, "failed to fetch build %d or buildInfra", buildID).Err())
	}

	// Create a backend task client
	backend, err := NewBackendClient(ctx, bld.Proto)
	if err != nil {
		return tq.Fatal.Apply(errors.Annotate(err, "failed to connect to backend service").Err())
	}

	taskReq, err := computeBackendNewTaskReq(ctx, bld, infra)
	if err != nil {
		return tq.Fatal.Apply(err)
	}

	// Create a backend task via RunTask
	_, err = backend.RunTask(ctx, taskReq)
	if err != nil {
		// Give up if HTTP 500s are happening continuously. Otherwise re-throw the
		// error so Cloud Tasks retries the task.
		if apiErr, _ := err.(*googleapi.Error); apiErr == nil || apiErr.Code >= 500 {
			if clock.Now(ctx).Sub(bld.CreateTime) < runTaskGiveUpTimeout {
				return transient.Tag.Apply(errors.Annotate(err, "failed to create a backend task").Err())
			}
			logging.Errorf(ctx, "Give up backend task creation retry after %s", runTaskGiveUpTimeout.String())
		}
		logging.Errorf(ctx, "Backend task creation failure:%s. RunTask request: %+v", err, taskReq)
		return tq.Fatal.Apply(errors.Annotate(err, "failed to create a backend task").Err())
	}

	return nil
}
