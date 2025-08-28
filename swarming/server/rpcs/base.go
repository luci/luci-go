// Copyright 2023 The LUCI Authors.
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

// Package rpcs implements public API RPC handlers.
package rpcs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cursor"
	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

var requestStateCtxKey = "swarming.rpcs.RequestState"

const (
	// Default size of a single page of results for listing queries.
	defaultPageSize = 100
	// Maximum allowed size of a single page of results for listing queries.
	maxPageSize = 1000
)

// SwarmingServer implements Swarming gRPC service.
//
// It is a collection of various RPCs that didn't fit other services. Individual
// RPCs are implemented in swarming_*.go files.
type SwarmingServer struct {
	apipb.UnimplementedSwarmingServer

	// ServerVersion is the version of the executing binary.
	ServerVersion string
}

// BotsServer implements Bots gRPC service.
//
// It exposes methods to view and manipulate state of Swarming bots. Individual
// RPCs are implemented in bots_*.go files.
type BotsServer struct {
	apipb.UnimplementedBotsServer

	// BotQuerySplitMode controls how "finely" to split BotInfo queries.
	BotQuerySplitMode model.SplitMode
	// BotsDimensionsCache caches aggregated bot dimensions sets.
	BotsDimensionsCache model.BotsDimensionsCache
	// TasksManager is used to change state of tasks.
	TasksManager tasks.Manager
}

// TasksServer implements Tasks gRPC service.
//
// It exposes methods to view and manipulate state of Swarming tasks. Individual
// RPCs are implemented in tasks_*.go files.
type TasksServer struct {
	apipb.UnimplementedTasksServer

	// TaskQuerySplitMode controls how "finely" to split TaskResultSummary queries.
	TaskQuerySplitMode model.SplitMode
	// TasksManager is used to change state of tasks.
	TasksManager tasks.Manager
}

// TaskBackend implements bbpb.TaskBackendServer.
type TaskBackend struct {
	bbpb.UnimplementedTaskBackendServer

	// BuildbucketTarget is "swarming://<swarming-cloud-project>".
	BuildbucketTarget string
	// BuildbucketAccount is the Buildbucket service account to expect calls from.
	BuildbucketAccount string
	// DisableBuildbucketCheck is true when running locally.
	DisableBuildbucketCheck bool
	// StatusPageLink produces links to task status pages.
	StatusPageLink func(taskID string) string

	// TasksServer is used to submit and cancel tasks.
	TasksServer *TasksServer
}

// CheckBuildbucket returns a gRPC error if the caller is not Buildbucket.
func (srv *TaskBackend) CheckBuildbucket(ctx context.Context) error {
	if srv.DisableBuildbucketCheck {
		return nil
	}

	state := auth.GetState(ctx)
	if state == nil {
		return status.Errorf(codes.Internal, "the auth state is not properly configured")
	}

	// Buildbucket is expected to use project identities: BB is the RPC peer, but
	// the request is authenticated as coming from a project.
	if peer := state.PeerIdentity(); peer.Kind() != identity.User || peer.Email() != srv.BuildbucketAccount {
		return status.Errorf(codes.PermissionDenied, "peer %q is not allowed to access this task backend", peer)
	}
	if user := state.User().Identity; user.Kind() != identity.Project {
		return status.Errorf(codes.PermissionDenied, "caller %q is not a project identity", user)
	}
	return nil
}

// RequestState carries stated scoped to a single RPC handler.
//
// In production produced by ServerInterceptor. In tests can be injected into
// the context via MockRequestState(...).
//
// Use State(ctx) to get the current value.
type RequestState struct {
	// Config is a snapshot of the server configuration when request started.
	Config *cfg.Config
	// ACL can be used to check ACLs.
	ACL *acls.Checker
}

// ServerInterceptor returns an interceptor that initializes per-RPC context.
//
// The interceptor is active only for selected gRPC services. All other RPCs
// are passed through unaffected.
//
// The initialized context will have RequestState populated, use State(ctx) to
// get it.
func ServerInterceptor(cfg *cfg.Provider, services []string) grpcutil.UnifiedServerInterceptor {
	serviceSet := stringset.NewFromSlice(services...)
	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
		// fullMethod looks like "/<service>/<method>". Get "<service>".
		if fullMethod == "" || fullMethod[0] != '/' {
			panic(fmt.Sprintf("unexpected fullMethod %q", fullMethod))
		}
		service := fullMethod[1:strings.LastIndex(fullMethod, "/")]
		if service == "" {
			panic(fmt.Sprintf("unexpected fullMethod %q", fullMethod))
		}

		if !serviceSet.Has(service) {
			return handler(ctx)
		}

		cfg := cfg.Cached(ctx)
		return handler(context.WithValue(ctx, &requestStateCtxKey, &RequestState{
			Config: cfg,
			ACL:    acls.NewChecker(ctx, cfg),
		}))
	}
}

// LegacyMiddleware initializes per-RPC context for legacy endpoints.
//
// Does the same thing as ServerInterceptor, but for legacy endpoints.
func LegacyMiddleware(cfg *cfg.Provider) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		ctx := c.Request.Context()
		cfg := cfg.Cached(ctx)
		c.Request = c.Request.WithContext(context.WithValue(ctx, &requestStateCtxKey, &RequestState{
			Config: cfg,
			ACL:    acls.NewChecker(ctx, cfg),
		}))
		next(c)
	}
}

// State accesses the per-request state in the context or panics if it is
// not there.
func State(ctx context.Context) *RequestState {
	state, _ := ctx.Value(&requestStateCtxKey).(*RequestState)
	if state == nil {
		panic("no RequestState in the context")
	}
	return state
}

// ValidateLimit validates a page size limit in listing queries.
func ValidateLimit(val int32) (int32, error) {
	if val == 0 {
		val = defaultPageSize
	}
	switch {
	case val < 0:
		return val, errors.Fmt("must be positive, got %d", val)
	case val > maxPageSize:
		return val, errors.Fmt("must be less or equal to %d, got %d", maxPageSize, val)
	}
	return val, nil
}

// CheckListingPerm checks the caller can perform a listing using the given tags
// or dimensions filter.
//
// It checks either the global ACL (if the filter doesn't specify any concrete
// pools to restrict the listing to) or ACLs of pools specified by the filter.
//
// Returns nil if it is OK to proceed or a gRPC error to stop. The error should
// be returned by the RPC as is.
func CheckListingPerm(ctx context.Context, filter model.Filter, perm realms.Permission) error {
	var res acls.CheckResult
	if pools := filter.Pools(); len(pools) != 0 {
		res = State(ctx).ACL.CheckAllPoolsPerm(ctx, pools, perm)
	} else {
		res = State(ctx).ACL.CheckServerPerm(ctx, perm)
	}
	if !res.Permitted {
		return res.ToGrpcErr()
	}
	return nil
}

// TaskListingRequest describes details of a task listing request.
type TaskListingRequest struct {
	// Permission that the caller must have in all requested pools.
	Perm realms.Permission
	// Start of the time range to query or nil if unlimited.
	Start *timestamppb.Timestamp
	// End of the time range to query or nil if unlimited.
	End *timestamppb.Timestamp
	// Filter on the task state.
	State apipb.StateQuery
	// How to sort the tasks.
	Sort apipb.SortQuery
	// Filter on the task tags.
	Tags []string
	// Serialized cursor.
	Cursor string
	// Expected kind of the cursor (ignored if there's no cursor).
	CursorKind cursorpb.RequestKind
	// How many entities each subquery can return at most (or 0 for unlimited).
	Limit int32
	// How to split complex queries into parallel queries.
	SplitMode model.SplitMode
}

// StartTaskListingRequest is a common part of all requests that list or count
// tasks with filtering by tags and state.
//
// It checks ACLs based on what pools are queried and prepares datastore queries
// (to be run in parallel) that return the matching TaskResultSummary entities.
//
// Returns gRPC errors. If the query is already known to produce no results at
// all (may happen when using time range filters with cursors), returns an empty
// list of queries.
func StartTaskListingRequest(ctx context.Context, req *TaskListingRequest) ([]*datastore.Query, error) {
	// Right now only SortQuery_QUERY_CREATED_TS is supported. Swarming lacks
	// necessary indices and actual functionality to support any other order. They
	// were never supported in ListTasks RPC, even in the older Python version.
	// They are partially supported in ListBotTasks RPC though (which doesn't use
	// this function).
	if req.Sort != apipb.SortQuery_QUERY_CREATED_TS {
		return nil, status.Errorf(codes.InvalidArgument, "sort order %q is not supported currently, open a bug if needed", req.Sort)
	}

	var dscursor *cursorpb.TasksCursor
	if req.Cursor != "" {
		var err error
		dscursor, err = cursor.Decode[cursorpb.TasksCursor](ctx, req.CursorKind, req.Cursor)
		if err != nil {
			return nil, err
		}
	}

	filter, err := model.NewFilterFromTags(req.Tags)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tags: %s", err)
	}
	if err := CheckListingPerm(ctx, filter, req.Perm); err != nil {
		return nil, err
	}

	var startTime time.Time
	if req.Start != nil {
		startTime = req.Start.AsTime()
	}
	var endTime time.Time
	if req.End != nil {
		endTime = req.End.AsTime()
	}

	// Limit to the requested time range (+ whatever range cursor is scanning).
	// An error here means the time range itself is invalid.
	query, err := model.FilterTasksByCreationTime(ctx,
		model.TaskResultSummaryQuery(),
		startTime,
		endTime,
		dscursor,
	)
	if err != nil {
		if errors.Is(err, datastore.ErrNullQuery) {
			return nil, nil
		}
		return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %s", err)
	}

	// This limit will be inherited by all subqueries below.
	if req.Limit != 0 {
		query = query.Limit(req.Limit)
	}

	// Limit to the requested state, if any. This may split the query into
	// multiple queries to be run in parallel. This can also update the split
	// mode to SplitCompletely if FilterTasksByState needs to add an IN filter,
	// see its doc.
	var stateQueries []*datastore.Query
	var splitMode model.SplitMode
	if req.State != apipb.StateQuery_QUERY_ALL {
		stateQueries, splitMode = model.FilterTasksByState(query, req.State, req.SplitMode)
	} else {
		stateQueries = []*datastore.Query{query}
		splitMode = req.SplitMode
	}

	// Filtering by tags may split the query even further. We'll need to merge
	// all resulting subqueries.
	var queries []*datastore.Query
	for _, query := range stateQueries {
		queries = append(queries, model.FilterTasksByTags(query, splitMode, filter)...)
	}
	return queries, nil
}

// FetchTaskRequest fetches a task request given its ID.
//
// Returns gRPC status errors, logs internal errors. Does not check ACLs yet.
// It is the caller's responsibility.
func FetchTaskRequest(ctx context.Context, taskID string) (*model.TaskRequest, error) {
	if taskID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task_id is required")
	}
	key, err := model.TaskIDToRequestKey(ctx, taskID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "task_id %s: %s", taskID, err)
	}
	switch req, err := model.FetchTaskRequest(ctx, key); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such task")
	case err != nil:
		logging.Errorf(ctx, "Error fetching TaskRequest %s: %s", taskID, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
	default:
		return req, nil
	}
}
