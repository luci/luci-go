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
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
)

func newSwarmbucketClient(c context.Context, server string) (*swarmbucket.Service, error) {
	c, _ = context.WithTimeout(c, time.Minute)
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, err
	}
	client, err := swarmbucket.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	client.BasePath = fmt.Sprintf("https://%s/api/swarmbucket/v1/", server)
	return client, nil
}

func newBuildbucketClient(c context.Context, server string) (*bbapi.Service, error) {
	c, _ = context.WithTimeout(c, time.Minute)
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, err
	}
	client, err := bbapi.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	client.BasePath = fmt.Sprintf("https://%s/api/buildbucket/v1/", server)
	return client, nil
}

func parseStatus(status buildbucket.Status) model.Status {
	// We map out non-infra failures explicitly. Any status not in this map
	// defaults to InfraFailure below.
	switch status {
	case buildbucket.StatusScheduled:
		return model.NotRun
	case buildbucket.StatusStarted:
		return model.Running
	case buildbucket.StatusSuccess:
		return model.Success
	case buildbucket.StatusFailure:
		return model.Failure
	case buildbucket.StatusError:
		return model.InfraFailure
	case buildbucket.StatusCancelled:
		return model.Cancelled
	case buildbucket.StatusTimeout:
		return model.Expired
	default:
		return model.InfraFailure
	}
}
