// Copyright 2020 The LUCI Authors.
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

package internals

import (
	"context"
	"fmt"
	"net/http"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/googleapis/gax-go/v2"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
)

// StatusOffError converts error, possibly nil, to the appropriate HTTP status
// code.
func StatusOffError(rctx *router.Context, err error) {
	kind := ""
	code := 0
	switch {
	case err == nil:
		rctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rctx.Writer.WriteHeader(200)
		fmt.Fprintln(rctx.Writer, "OK")
		return
	case transient.Tag.In(err):
		kind = "transient"
		// Unlike 503, 429 doesn't throttle the Cloud Task queue execution rate.
		code = 429
	default:
		kind = "permanent"
		code = 202 // avoid retries.
	}
	logging.Errorf(rctx.Context, "HTTP %d: %s error: %s", code, kind, err)
	errors.Log(rctx.Context, err)
	http.Error(rctx.Writer, "", code)
}

// UnborkTasksClient works around cloudtasks.Client bug.
//
// WORKAROUND(https://github.com/googleapis/google-cloud-go/issues/1577):
// if the passed context deadline is larger than 30s, the CreateTask call fails
// with InvalidArgument "The request deadline is ... The deadline cannot be more
// than 30s in the future.". So, give it 20s.
//
// If passed object isn't a cloudtasks.Client instance, then returns the object
// itself.
func UnborkTasksClient(c TasksClient) TasksClient {
	if buggy, ok := c.(*cloudtasks.Client); ok {
		return unborkedTasksClient{buggy}
	}
	return c
}

type unborkedTasksClient struct {
	prod *cloudtasks.Client
}

func (u unborkedTasksClient) CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest, opts ...gax.CallOption) (*taskspb.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*20)
	defer cancel()
	return u.prod.CreateTask(ctx, req)
}
