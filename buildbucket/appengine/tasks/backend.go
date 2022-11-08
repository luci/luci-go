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
	"strconv"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

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
	_, err := computeBackendNewTaskReq(ctx, bld, infra)
	if err != nil {
		return tq.Fatal.Apply(err)
	}

	return errors.Reason("Method not implemented").Err()
}
