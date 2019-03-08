// Copyright 2016 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"fmt"
	"net/http"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
)

const bbRPCTimeout = time.Minute

func newSwarmbucketClient(c context.Context, server string) (*swarmbucket.Service, error) {
	c, cancel := context.WithTimeout(c, bbRPCTimeout)
	defer cancel()
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, err
	}
	client, err := swarmbucket.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	client.BasePath = fmt.Sprintf("https://%s/_ah/api/swarmbucket/v1/", server)
	return client, nil
}

func newBuildbucketClient(c context.Context, server string) (*bbv1.Service, error) {
	c, cancel := context.WithTimeout(c, bbRPCTimeout)
	defer cancel()
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, err
	}
	client, err := bbv1.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	client.BasePath = fmt.Sprintf("https://%s/_ah/api/buildbucket/v1/", server)
	return client, nil
}

// statusMap maps buildbucket status to milo status.
// Buildbucket statuses not in the map must be treated
// as InfraFailure.
var statusMap = map[buildbucketpb.Status]model.Status{
	buildbucketpb.Status_SCHEDULED:     model.NotRun,
	buildbucketpb.Status_STARTED:       model.Running,
	buildbucketpb.Status_SUCCESS:       model.Success,
	buildbucketpb.Status_FAILURE:       model.Failure,
	buildbucketpb.Status_INFRA_FAILURE: model.InfraFailure,
	buildbucketpb.Status_CANCELED:      model.Cancelled,
}

// parseStatus converts a buildbucket status to model.Status.
func parseStatus(status buildbucketpb.Status) model.Status {
	if st, ok := statusMap[status]; ok {
		return st
	}
	return model.InfraFailure
}
