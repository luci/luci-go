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

package join

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
)

// JoinRootInvocation notifies ingestion that the given root invocation has
// finalized.
// Ingestion tasks are created when all required data for a ingestion
// (including any associated LUCI CV run, build and invocation) is available.
func JoinRootInvocation(ctx context.Context, notification *rdbpb.RootInvocationFinalizedNotification) (processed bool, err error) {
	rootInvocation := notification.GetRootInvocation()
	if rootInvocation == nil {
		return false, errors.New("root invocation must be specified")
	}

	project, _ := realms.Split(rootInvocation.Realm)
	_, err = pbutil.ParseRootInvocationName(rootInvocation.Name)
	if err != nil {
		return false, errors.Fmt("parse root invocation name: %w", err)
	}

	result := &controlpb.RootInvocationResult{
		ResultdbHost:   rdbHost,
		RootInvocation: rootInvocation,
	}
	// TODO: Implement the actual root invocation join functionality.
	logging.Infof(ctx, "Successfully joined the root invocation: %v for project: %s", result, project)
	return true, nil
}
