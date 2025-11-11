// Copyright 2025 The LUCI Authors.
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

package pubsub

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/tasks"
)

type Options struct {
	// Hostname of the luci.resultdb.v1.ResultDB service which can be
	// queried to fetch the details of invocations being exported.
	// E.g. "results.api.cr.dev".
	ResultDBHostname string
}

// InitServer initializes a exportnotifier server.
func InitServer(srv *server.Server, opts Options) {
	tasks.TestResultsPublisher.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		return handlePublishTestResultsTask(ctx, msg, opts.ResultDBHostname)
	})
}
