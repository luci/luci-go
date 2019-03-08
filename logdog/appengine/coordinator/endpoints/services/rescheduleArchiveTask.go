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

// TODO(hinoka): Remove after crbug.com/923557

package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/mutations"
	"go.chromium.org/luci/tumble"

	"google.golang.org/grpc/codes"
)

const maxDelay = 2 * time.Hour

// RecheduleArchiveTask schedules an archive task for a log stream.
func (b *server) RescheduleArchiveTask(c context.Context, req *logdog.ArchiveDispatchTask) (*empty.Empty, error) {
	logging.Debugf(c, "Received request for stream %s", req.Id)
	// Verify that the request is minimially valid.
	if req.Id == "" {
		return nil, grpcutil.Errf(codes.InvalidArgument, "missing id")
	}
	id := coordinator.HashID(req.Id)

	// Do some sanity checks to make sure this archival makes sense.
	lst := coordinator.NewLogStreamState(c, id)
	switch err := datastore.Get(c, lst); err {
	case nil:
		logging.Debugf(c, "Found log stream %s", lst)
		// OK
	case datastore.ErrNoSuchEntity:
		return &empty.Empty{}, grpcutil.Errf(codes.NotFound, "Log Stream not found")
	default:
		return &empty.Empty{}, err
	}

	delay := time.Second * time.Duration(math.Pow(2.0, float64(lst.ArchiveRetryCount)))
	if delay > maxDelay {
		delay = maxDelay
	}

	cat := mutations.CreateArchiveTask{
		ID:         id,
		Key:        req.Key,
		Expiration: clock.Now(c).Add(delay),
	}
	lstKey := datastore.KeyForObj(c, lst)
	aeName := cat.TaskName(c)
	if len(lst.ArchivalKey) > 0 {
		// Give retries unique names, so that they don't dedupe.
		aeName = fmt.Sprintf("%s-%d", aeName, lst.ArchiveRetryCount)
	}
	return &empty.Empty{}, tumble.PutNamedMutations(c, lstKey, map[string]tumble.Mutation{aeName: &cat})
}
