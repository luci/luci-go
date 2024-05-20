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

package joinlegacy

import (
	"context"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/ingestion/controllegacy"
)

// JoinInvocation notifies ingestion that the given invocation has finalized.
// Ingestion tasks are created for buildbucket builds when all required data
// for a build (including any invocation) is available.
func JoinInvocation(ctx context.Context, notification *rdbpb.InvocationFinalizedNotification) (processed bool, err error) {
	project, _ := realms.Split(notification.Realm)
	id, err := pbutil.ParseInvocationName(notification.Invocation)
	if err != nil {
		return false, errors.Annotate(err, "parse invocation name").Err()
	}

	match := buildInvocationRE.FindStringSubmatch(id)
	if match == nil {
		// Invocations that are not of the form build-<BUILD ID> are ignored.
		return false, nil
	}
	bbBuildID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return false, errors.Annotate(err, "parse build ID").Err()
	}

	// This proto has no fields for now. If we need to pass anything about the invocation,
	// we can add it in here in future.
	result := &controlpb.InvocationResult{}

	buildID := controllegacy.BuildID(bbHost, bbBuildID)
	if err := JoinInvocationResult(ctx, buildID, project, result); err != nil {
		return true, errors.Annotate(err, "joining invocation result").Err()
	}
	return true, nil
}
