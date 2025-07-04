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

package rbe

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/tq"

	notificationspb "go.chromium.org/luci/swarming/internal/notifications"
	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

const (
	// How many times to retry submitting a reservation in case the bot dies right
	// after picking it up.
	maxReservationRetryCount = 3
)

// Incremented when a failed RBE reservation is resubmitted.
var resubmitCount = metric.NewCounter(
	"swarming/rbe/resubmissions",
	"Counter of RBE reservation resubmissions",
	&types.MetricMetadata{},
	// Swarming pool of the resubmitted reservation.
	field.String("pool"),
)

// ErrBadReservation is wrapped and returned by EnqueueNew if the reservation
// fails to pass minimal validation checks using the current version of
// the server config.
//
// This should be rare and happen only for reservations that are being retried,
// since completely new reservations are validated before EnqueueNew call. When
// reservations are retried, the server config may be different already from
// what it was when the reservation was created initially.
var ErrBadReservation = errors.New("the reservation cannot be submitted")

// ReservationServer is responsible for creating and canceling RBE reservations.
type ReservationServer struct {
	rbe           remoteworkers.ReservationsClient
	internals     internalspb.InternalsClient
	disp          *tq.Dispatcher // initialized in RegisterTQTasks
	serverProject string
	serverVersion string
	cfg           *cfg.Provider
	// tasksManager is used to change state of tasks.
	tasksManager tasks.Manager

	// enqueueNew is usually tasks.EnqueueRBENew, but it can be mocked in tests.
	enqueueNew func(context.Context, *tq.Dispatcher, *model.TaskRequest, *model.TaskToRun, *cfg.Config) error
}

// NewReservationServer creates a new reservation server given an RBE client
// connection.
func NewReservationServer(ctx context.Context, cc grpc.ClientConnInterface, internals internalspb.InternalsClient, serverProject, serverVersion string, cfg *cfg.Provider, tasksManager tasks.Manager) *ReservationServer {
	return &ReservationServer{
		rbe:           remoteworkers.NewReservationsClient(cc),
		internals:     internals,
		serverProject: serverProject,
		serverVersion: serverVersion,
		cfg:           cfg,
		tasksManager:  tasksManager,
		enqueueNew:    tasks.EnqueueRBENew,
	}
}

// RegisterTQTasks registers task queue handlers.
func (s *ReservationServer) RegisterTQTasks(tasks *tqtasks.Tasks) {
	s.disp = tasks.TQ
	tasks.EnqueueRBE.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		return s.handleEnqueueRBETask(ctx, payload.(*internalspb.EnqueueRBETask))
	})
	tasks.CancelRBE.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		return s.handleCancelRBETask(ctx, payload.(*internalspb.CancelRBETask))
	})
}

// RegisterPSHandlers registers PubSub handlers.
func (s *ReservationServer) RegisterPSHandlers(disp *pubsub.Dispatcher) {
	disp.RegisterHandler("rbe-scheduler", pubsub.WirePB(
		func(ctx context.Context, md pubsub.Message, m *notificationspb.SchedulerNotification) (err error) {
			// Log details of the message that caused the error.
			defer func() {
				if err != nil {
					logging.Infof(ctx, "MessageID: %s", md.MessageID)
					logging.Infof(ctx, "Subscription: %s", md.Subscription)
					logging.Infof(ctx, "PublishTime: %s", md.PublishTime)
					logging.Infof(ctx, "Attributes: %v", md.Attributes)
					logging.Infof(ctx, "Query: %s", md.Query)
					bodyPB, _ := prototext.Marshal(m)
					logging.Infof(ctx, "Message body:\n%s", bodyPB)
				}
			}()
			projectID := md.Attributes["project_id"]
			if projectID == "" {
				return errors.New("no project_id message attribute")
			}
			instanceID := md.Attributes["instance_id"]
			if instanceID == "" {
				return errors.New("no instance_id message attribute")
			}
			if m.ReservationId == "" {
				return errors.New("reservation_id is unexpectedly empty")
			}
			reservationName := fmt.Sprintf("projects/%s/instances/%s/reservations/%s",
				projectID,
				instanceID,
				m.ReservationId,
			)
			return s.expireSliceBasedOnReservation(ctx, reservationName)
		},
	))
}

// handleEnqueueRBETask is responsible for creating a reservation.
func (s *ReservationServer) handleEnqueueRBETask(ctx context.Context, task *internalspb.EnqueueRBETask) (err error) {
	// On fatal errors need to mark the TaskToRun in Swarming DB as failed. On
	// transient errors just let the TQ to call us again for a retry. Note that
	// CreateReservation will fail with AlreadyExists if we actually managed to
	// create the reservation on the previous attempt.
	defer func() {
		if err != nil && !transient.Tag.In(err) {
			// Something is fatally broken, report this to Swarming.
			if derr := s.reservationDenied(ctx, task.Payload, err); derr != nil {
				// If the report itself failed for whatever reason, ask TQ to retry by
				// returning this error as is (i.e. not tagged with Fatal or Ignore).
				err = derr
			} else {
				// Swarming was successfully notified and we need to tell TQ not to
				// retry the task by tagging the error. Mark expected fatal errors (like
				// FailedPrecondition due to missing bots) with Ignore, and all
				// unexpected ones (e.g. PermissionDenied due to misconfiguration) with
				// Fatal tag. This will make them show up differently in monitoring and
				// logs.
				if isExpectedRBEError(err) {
					err = tq.Ignore.Apply(err)
				} else {
					err = tq.Fatal.Apply(err)
				}
			}
		}
	}()

	// Fetch TaskToRun and verify it is still pending. It may have been canceled
	// already. This check is especially important if handleEnqueueRBETask is
	// being retried in a loop due to some repeating error. Canceling TaskToRun
	// should stop this retry loop.
	ttr, err := newTaskToRunFromPayload(ctx, task.Payload)
	if err != nil {
		return errors.Fmt("bad EnqueueRBETask payload: %w", err)
	}
	switch err := datastore.Get(ctx, ttr); {
	case err == datastore.ErrNoSuchEntity:
		logging.Warningf(ctx, "TaskToRun entity is already gone")
		return nil
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to fetch TaskToRun: %w", err))
	case !ttr.IsReapable():
		logging.Warningf(ctx, "TaskToRun is no longer pending")
		return nil
	}

	// This will show up in e.g. Swarming bot logs.
	if task.Payload.DebugInfo != nil {
		task.Payload.DebugInfo.GoSwarmingVersion = s.serverVersion
	}

	payload, err := anypb.New(task.Payload)
	if err != nil {
		return errors.Fmt("failed to serialize the payload: %w", err)
	}

	constraints := make([]*remoteworkers.Constraint, len(task.Constraints))
	for i, ctr := range task.Constraints {
		constraints[i] = &remoteworkers.Constraint{
			Key:           "label:" + ctr.Key,
			AllowedValues: ctr.AllowedValues,
		}
	}

	// How much time left to sit in the queue.
	untilExpired := task.Expiry.AsTime().Sub(clock.Now(ctx))
	if untilExpired < 0 {
		untilExpired = 0
	}
	queuingTimeout := durationpb.New(untilExpired)
	// How much time left to wait for a matching bot.
	var matchTimeout *durationpb.Duration
	if task.WaitForCapacity {
		matchTimeout = queuingTimeout
	}
	// How much time there is to run once started.
	executionTimeout := task.ExecutionTimeout
	// The task must be done by that time.
	overallExpiration := timestamppb.New(
		task.Expiry.AsTime().Add(task.ExecutionTimeout.AsDuration()))

	logging.Infof(ctx, "Creating reservation %q", task.Payload.ReservationId)
	reservationName := fmt.Sprintf("%s/reservations/%s", task.RbeInstance, task.Payload.ReservationId)
	reservation, err := s.rbe.CreateReservation(ctx, &remoteworkers.CreateReservationRequest{
		Parent: task.RbeInstance,
		Reservation: &remoteworkers.Reservation{
			Name:             reservationName,
			Payload:          payload,
			ExpireTime:       overallExpiration,
			QueuingTimeout:   queuingTimeout,
			ExecutionTimeout: executionTimeout,
			Priority:         task.Priority,
			Constraints:      constraints,
			RequestedBotId:   task.RequestedBotId,
			MatchTimeout:     matchTimeout,
		},
	})
	if status.Code(err) == codes.AlreadyExists {
		logging.Warningf(ctx, "Reservation already exists, this is likely a retry: %s", err)
		reservation, err = s.rbe.GetReservation(ctx, &remoteworkers.GetReservationRequest{
			Name:        reservationName,
			WaitSeconds: 0,
		})
	}

	// Ask TQ to retry on transient gRPC errors.
	err = grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
	if err != nil {
		return err
	}

	logging.Infof(ctx, "Reservation: %v", reservation)
	return nil
}

// reservationDenied notifies the Swarming Python side that the reservation
// cannot be created due to some fatal error (in particular if there are no
// RBE bots that can execute it).
func (s *ReservationServer) reservationDenied(ctx context.Context, task *internalspb.TaskPayload, reason error) error {
	level := logging.Error
	if isExpectedRBEError(reason) {
		level = logging.Warning
	}
	logging.Logf(ctx, level, "Failed to submit RBE reservation %q: %s", task.ReservationId, reason)

	// Convert RBE reply to a slice expiration reason. Note that there's
	// specifically no generic "UNKNOWN" error: all possible RBE errors should
	// have known reasons.
	var reasonCode internalspb.ExpireSliceRequest_Reason
	switch grpcutil.Code(reason) {
	case codes.FailedPrecondition:
		// There are no bots alive matching the task.
		reasonCode = internalspb.ExpireSliceRequest_NO_RESOURCE
	case codes.ResourceExhausted:
		// QueuingTimeout is 0 and there are no non-busy bots matching the task.
		reasonCode = internalspb.ExpireSliceRequest_EXPIRED
	case codes.PermissionDenied:
		// Likely an RBE instance misconfiguration (i.e. should not happen).
		reasonCode = internalspb.ExpireSliceRequest_PERMISSION_DENIED
	case codes.InvalidArgument:
		// RBE doesn't like format of dimensions (i.e. should not happen).
		reasonCode = internalspb.ExpireSliceRequest_INVALID_ARGUMENT
	default:
		return errors.Fmt("unexpected RBE gRPC status code in %s", reason)
	}

	// Tell Swarming to switch to the next slice, if necessary.
	return s.expireSlice(ctx, task, reasonCode, "", reason.Error())
}

// expireSliceBasedOnReservation checks the reservation status by calling
// the Reservations API and invokes ExpireSlice if the reservation is dead.
//
// It is ultimately invoked from a PubSub push handler when handling
// notifications from the RBE scheduler.
//
// `reservationName` is a full reservation name, including the project and
// RBE instance IDs: `projects/.../instances/.../reservations/...`.
func (s *ReservationServer) expireSliceBasedOnReservation(ctx context.Context, reservationName string) error {
	// Get the up-to-date state of the reservation.
	reservation, err := s.rbe.GetReservation(ctx, &remoteworkers.GetReservationRequest{
		Name:        reservationName,
		WaitSeconds: 0,
	})
	err = grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
	if err != nil {
		return errors.Fmt("failed to fetch reservation %s: %w", reservationName, err)
	}

	// Don't care about pending reservations.
	if reservation.State != remoteworkers.ReservationState_RESERVATION_COMPLETED &&
		reservation.State != remoteworkers.ReservationState_RESERVATION_CANCELLED {
		logging.Warningf(ctx, "Ignoring reservation %s in non-terminal state: %s", reservationName, reservation.State)
		return nil
	}

	// Get the final reservation status. Note that if a bot picked up a lease and
	// then failed it, the status is still OK and the error is propagated through
	// the `result` field.
	//
	// Observed non-OK statuses:
	//   * FAILED_PRECONDITION if there are no matching bots at all.
	//   * DEADLINE_EXCEEDED if the reservation wasn't finished before its expiry.
	//   * INTERNAL if the reservation was completed, but with unset result field.
	//   * CANCELLED if the reservation was canceled before being assigned.
	//   * ABORTED if the bot picked up the lease and then stopped sending pings.
	//
	// Note that canceling the reservation with the bot's acknowledgment (i.e.
	// while it is already running) results in OK status.
	statusErr := status.ErrorProto(reservation.Status)
	if statusErr == nil && reservation.State == remoteworkers.ReservationState_RESERVATION_CANCELLED {
		statusErr = status.Error(codes.Canceled, "reservation was canceled")
	}

	// TaskPayload contains exact "coordinates" of the TaskToRun in the datastore.
	// It must be present in all leases.
	var payload internalspb.TaskPayload
	if err := reservation.Payload.UnmarshalTo(&payload); err != nil {
		return errors.Fmt("failed to unmarshal reservation %s payload: %w", reservationName, err)
	}
	logging.Infof(ctx, "TaskPayload:\n%s", prettyProto(&payload))

	// TaskResult contains extra information supplied by the bot when it was
	// closing the lease. It is empty if the lease didn't reach the bot at all.
	if reservation.Result != nil {
		var result internalspb.TaskResult
		if err := reservation.Result.UnmarshalTo(&result); err != nil {
			return errors.Fmt("failed to unmarshal reservation result: %w", err)
		}
		if result.BotInternalError != "" {
			if statusErr != nil {
				logging.Errorf(ctx, "Overriding with a bot internal error: %s", statusErr)
			}
			statusErr = status.Errorf(codes.Internal, "%s: %s", reservation.AssignedBotId, result.BotInternalError)
		}
	}

	// Log the final derived status.
	if statusErr != nil {
		logging.Infof(ctx, "Reservation is %s by %q: %s", reservation.State, reservation.AssignedBotId, statusErr)
	} else {
		logging.Infof(ctx, "Reservation is %s by %q", reservation.State, reservation.AssignedBotId)
	}

	// Ignore noop reservations: they are not associated with Swarming slices and
	// used during manual testing only.
	if payload.Noop {
		return nil
	}

	// See if the TaskToRun is still pending. If it was already assigned or
	// canceled per Swarming datastore state, there's nothing to do.
	ttr, err := newTaskToRunFromPayload(ctx, &payload)
	if err != nil {
		return errors.Fmt("bad TaskPayload: %w", err)
	}
	switch err := datastore.Get(ctx, ttr); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Warningf(ctx, "TaskToRun entity is already gone")
		return nil
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to fetch TaskToRun: %w", err))
	case !ttr.IsReapable():
		return nil
	case ttr.RBEReservation != payload.ReservationId:
		logging.Warningf(ctx,
			"Skipping stale notification: expecting reservation %q, but got notification about %q",
			ttr.RBEReservation, payload.ReservationId)
		return nil
	}

	// We already checked the TaskToRun slice is still pending, which means the
	// bot hasn't called `/bot/claim` yet and haven't started working on a task
	// yet (it doesn't know what task to work on). But if `statusErr` is nil, the
	// reservation is already finished (and successfully at that). This should not
	// be possible and indicates some buggy or non-compliant bot.
	if statusErr == nil {
		logging.Errorf(ctx, "Unexpected completion notification")
		statusErr = status.Errorf(codes.Internal, "the reservation is unexpectedly finished by %q", reservation.AssignedBotId)
	}

	// Convert an RBE error condition to a slice expiration status.
	var reasonCode internalspb.ExpireSliceRequest_Reason
	switch status.Code(statusErr) {
	case codes.FailedPrecondition:
		reasonCode = internalspb.ExpireSliceRequest_NO_RESOURCE
	case codes.DeadlineExceeded:
		reasonCode = internalspb.ExpireSliceRequest_EXPIRED
	case codes.Internal, codes.Aborted:
		reasonCode = internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR
	default:
		// Note that this branch includes codes.Canceled which happens when
		// a reservation is canceled before it is assigned to a bot. Currently the
		// cancellation is always initiated through Swarming and Swarming already
		// marks the corresponding TaskToRun as consumed before canceling the
		// reservation: we should never end up here due to ttr.IsReapable() check
		// above.
		logging.Errorf(ctx, "Ignoring unexpected reservation status: %s", statusErr)
		return nil
	}

	// If this reservation was picked up by a bot and later dropped before it
	// was claimed (i.e. before the bot actually started working on it, we already
	// checked that above), we can resubmit it as a new reservation.
	if reasonCode == internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR {
		if ttr.RetryCount < maxReservationRetryCount {
			logging.Warningf(ctx, "Resubmitting reservation (retry #%d)", ttr.RetryCount+1)
			switch err := s.resubmitReservation(ctx, ttr.Key, &payload); {
			case err == nil:
				return nil
			case errors.Is(err, ErrBadReservation):
				logging.Errorf(ctx, "Giving up: %s", err)
				reasonCode = internalspb.ExpireSliceRequest_EXPIRED
				statusErr = err
			default:
				logging.Warningf(ctx, "Transient error resubmitting reservation: %s", err)
				return transient.Tag.Apply(err)
			}
		} else {
			logging.Warningf(ctx, "The reservation was retried too many times, giving up")
		}
	}

	// Tell Swarming to switch to the next slice, if necessary
	logging.Warningf(ctx, "Expiring slice with %s: %s", reasonCode, statusErr)
	return s.expireSlice(ctx, &payload, reasonCode, reservation.AssignedBotId, statusErr.Error())
}

func (s *ReservationServer) expireSlice(ctx context.Context, task *internalspb.TaskPayload, code internalspb.ExpireSliceRequest_Reason, culpritRBEBotID, details string) error {
	// Use `/prpc/swarming.internals.rbe.Internals/ExpireSlice` as the pseudo
	// route for traffic migration, even though the Go server uses
	// tasks.ExpireSliceTxn instead of the real RPC to expire slice.
	percent := s.cfg.Cached(ctx).RouteToGoPercent("/prpc/swarming.internals.rbe.Internals/ExpireSlice")
	if percent != 0 && (percent == 100 || mathrand.Intn(ctx, 100) < percent) {
		// Do not log when 100% of requests are configured to be routed to Go, this
		// is the default end state. Just absence of this logging line is good
		// enough indicator that requests are handled by Go.
		if percent != 100 {
			logging.Infof(ctx, "Routing to Go to expire slice (configured to route %d%% to Go)", percent)
		}
		return s.expireSliceGo(ctx, task, code, culpritRBEBotID)
	}
	logging.Infof(ctx, "Routing to Python to expire slice (configured to route %d%% to Go)", percent)
	return s.expireSlicePy(ctx, task, code, culpritRBEBotID, details)
}

// expireSlicePy calls Swarming Python's ExpireSlice RPC.
// TODO(b/355013802): delete after task slice expiration is fully migrated to Go.
func (s *ReservationServer) expireSlicePy(ctx context.Context, task *internalspb.TaskPayload, code internalspb.ExpireSliceRequest_Reason, culpritRBEBotID, details string) error {
	// Do not use effective bot ID in place of the real bot ID.
	if _, _, _, ok := model.ParseRBEEffectiveBotID(culpritRBEBotID); ok {
		culpritRBEBotID = ""
	}
	_, err := s.internals.ExpireSlice(ctx, &internalspb.ExpireSliceRequest{
		TaskId:         task.TaskId,
		TaskToRunShard: task.TaskToRunShard,
		TaskToRunId:    task.TaskToRunId,
		Reason:         code,
		Details:        details,
		CulpritBotId:   culpritRBEBotID,
	})
	return err
}

// expireSliceGo calls tasksManager.ExpireSliceTxn.
func (s *ReservationServer) expireSliceGo(ctx context.Context, task *internalspb.TaskPayload, code internalspb.ExpireSliceRequest_Reason, culpritRBEBotID string) error {
	// Do not use effective bot ID in place of the real bot ID.
	if _, _, _, ok := model.ParseRBEEffectiveBotID(culpritRBEBotID); ok {
		culpritRBEBotID = ""
	}

	reqKey, err := model.TaskIDToRequestKey(ctx, task.TaskId)
	if err != nil {
		return err
	}
	tr, err := model.FetchTaskRequest(ctx, reqKey)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return errors.New("TaskRequest not found")
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to fetch TaskRequest: %w", err))
	}

	var reason tasks.ExpireReason
	switch code {
	case internalspb.ExpireSliceRequest_NO_RESOURCE:
		reason = tasks.NoResource
	case internalspb.ExpireSliceRequest_PERMISSION_DENIED:
		reason = tasks.PermissionDenied
	case internalspb.ExpireSliceRequest_INVALID_ARGUMENT:
		reason = tasks.InvalidArgument
	case internalspb.ExpireSliceRequest_BOT_INTERNAL_ERROR:
		reason = tasks.BotInternalError
	case internalspb.ExpireSliceRequest_EXPIRED:
		reason = tasks.Expired
	default:
		panic(fmt.Sprintf("unexpected expire slice code %d", code))
	}
	expireOp := &tasks.ExpireSliceOp{
		Request:      tr,
		ToRunKey:     model.TaskToRunKey(ctx, tr.Key, task.TaskToRunShard, task.TaskToRunId),
		Reason:       reason,
		CulpritBotID: culpritRBEBotID,
		Config:       s.cfg.Cached(ctx),
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		_, err := s.tasksManager.ExpireSliceTxn(ctx, expireOp)
		return err
	}, nil)
	if err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to expire slice: %w", err))
	}
	return nil
}

// resubmitReservation submits a new RBE reservation for the given TaskToRun.
//
// Returns:
//   - nil if successfully submitted the reservation or it is no longer needed.
//   - a wrapped ErrBadReservation if the reservation is "broken" now and cannot
//     be resubmitted. This can happen if the server config changes.
//   - all other errors can be considered transient.
func (s *ReservationServer) resubmitReservation(ctx context.Context, ttr *datastore.Key, prev *internalspb.TaskPayload) error {
	tr, err := model.FetchTaskRequest(ctx, ttr.Parent())
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Warningf(ctx, "TaskRequest entity is already gone")
		return nil
	case err != nil:
		return errors.Fmt("failed to fetch TaskRequest: %w", err)
	}

	submitted := false
	err = datastore.RunInTransaction(ctx, func(tctx context.Context) error {
		submitted = false

		// Refetch TaskToRun and ensure it was untouched.
		ttr := &model.TaskToRun{Key: ttr}
		switch err := datastore.Get(tctx, ttr); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			logging.Warningf(tctx, "TaskToRun entity is already gone")
			return nil
		case err != nil:
			return errors.Fmt("failed to fetch TaskToRun: %w", err)
		case !ttr.IsReapable():
			logging.Warningf(tctx, "TaskToRun is no longer pending")
			return nil
		case ttr.RBEReservation != prev.ReservationId:
			logging.Warningf(tctx, "The reservation was already resubmitted")
			return nil
		}

		// Report this to monitoring (if the transaction actually lands).
		submitted = true

		// Submit the retry.
		ttr.RetryCount++
		ttr.RBEReservation = model.NewReservationID(s.serverProject, ttr.Key.Parent(), ttr.TaskSliceIndex(), int(ttr.RetryCount))
		if err := datastore.Put(tctx, ttr); err != nil {
			return errors.Fmt("failed to store TaskToRun: %w", err)
		}
		return s.enqueueNew(tctx, s.disp, tr, ttr, s.cfg.Cached(ctx))
	}, nil)

	if err != nil {
		return err
	}

	if submitted {
		pool := tr.Pool()
		if pool == "" {
			pool = "<unknown>"
		}
		resubmitCount.Add(ctx, 1, pool)
	}

	return nil
}

// handleCancelRBETask cancels a reservation.
func (s *ReservationServer) handleCancelRBETask(ctx context.Context, task *internalspb.CancelRBETask) error {
	logging.Infof(ctx, "Cancelling reservation %q", task.ReservationId)
	resp, err := s.rbe.CancelReservation(ctx, &remoteworkers.CancelReservationRequest{
		Name:   fmt.Sprintf("%s/reservations/%s", task.RbeInstance, task.ReservationId),
		Intent: remoteworkers.CancelReservationIntent_ANY,
	})
	switch status.Code(err) {
	case codes.OK:
		logging.Infof(ctx, "Cancel result: %s", resp.Result)
		return nil
	case codes.NotFound:
		logging.Warningf(ctx, "No such reservation, nothing to cancel")
		return tq.Ignore.Apply(err)
	default:
		return grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
	}
}

// newTaskToRunFromPayload returns an empty TaskToRun struct with populated
// entity key using information from the TaskPayload proto.
func newTaskToRunFromPayload(ctx context.Context, p *internalspb.TaskPayload) (*model.TaskToRun, error) {
	taskReqKey, err := model.TaskIDToRequestKey(ctx, p.TaskId)
	if err != nil {
		return nil, err
	}
	return &model.TaskToRun{
		Key: model.TaskToRunKey(ctx, taskReqKey, p.TaskToRunShard, p.TaskToRunId),
	}, nil
}

// isExpectedRBEError returns true for errors that are expected to happen during
// normal code flow.
func isExpectedRBEError(err error) bool {
	code := grpcutil.Code(err)
	return code == codes.FailedPrecondition || code == codes.ResourceExhausted
}

// prettyProto formats a proto message for logs.
func prettyProto(msg proto.Message) string {
	blob, err := prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(msg)
	if err != nil {
		return fmt.Sprintf("<error: %s>", err)
	}
	return string(blob)
}
