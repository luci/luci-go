// Copyright 2024 The LUCI Authors.
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

package join

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
)

const (
	// TODO: Removing the hosts after ResultDB PubSub, CVPubSub and GetRun RPC added them.
	// Host name of ResultDB.
	rdbHost = "results.api.luci.app"
)

// JoinInvocation notifies ingestion that the given invocation has finalized.
// Ingestion tasks are created when all required data for a ingestion
// (including any associated LUCI CV run, build and invocation) is available.
func JoinInvocation(ctx context.Context, notification *rdbpb.InvocationFinalizedNotification) (processed bool, err error) {
	project, _ := realms.Split(notification.Realm)
	id, err := pbutil.ParseInvocationName(notification.Invocation)
	if err != nil {
		return false, errors.Fmt("parse invocation name: %w", err)
	}

	if !isBuildbucketBuildInvocation(id) && !notification.IsExportRoot {
		// Invocations that are not associated with a buildbucket build and not a export root are ignored.
		return false, nil
	}

	result := &controlpb.InvocationResult{
		ResultdbHost: rdbHost,
		InvocationId: id,
		CreationTime: timestamppb.New(notification.CreateTime.AsTime()),
	}
	if err := JoinInvocationResult(ctx, id, project, result); err != nil {
		return true, errors.Fmt("joining invocation result: %w", err)
	}
	return true, nil
}

// Invocation is a buildbucket build invocation if invocation id is of the form build-<BUILD ID>.
func isBuildbucketBuildInvocation(invocationID string) bool {
	return buildInvocationRE.MatchString(invocationID)
}
