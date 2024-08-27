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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/model"

	// Enable datastore transactional tasks support.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

// ReservationServer is responsible for creating and canceling RBE reservations.
type ReservationServer struct {
	rbe           remoteworkers.ReservationsClient
	internals     internalspb.InternalsClient
	serverVersion string
}

// NewReservationServer creates a new reservation server given an RBE client
// connection.
func NewReservationServer(ctx context.Context, cc grpc.ClientConnInterface, internals internalspb.InternalsClient, serverVersion string) *ReservationServer {
	return &ReservationServer{
		rbe:           remoteworkers.NewReservationsClient(cc),
		internals:     internals,
		serverVersion: serverVersion,
	}
}

// RegisterTQTasks registers task queue handlers.
func (s *ReservationServer) RegisterTQTasks(disp *tq.Dispatcher) {
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "rbe-enqueue",
		Prototype: &internalspb.EnqueueRBETask{},
		Kind:      tq.Transactional,
		Queue:     "rbe-enqueue",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return s.handleEnqueueRBETask(ctx, payload.(*internalspb.EnqueueRBETask))
		},
	})
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "rbe-cancel",
		Prototype: &internalspb.CancelRBETask{},
		Kind:      tq.Transactional,
		Queue:     "rbe-cancel",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return s.handleCancelRBETask(ctx, payload.(*internalspb.CancelRBETask))
		},
	})
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
		return errors.Annotate(err, "bad EnqueueRBETask payload").Err()
	}
	switch err := datastore.Get(ctx, ttr); {
	case err == datastore.ErrNoSuchEntity:
		logging.Warningf(ctx, "TaskToRun entity is already gone")
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to fetch TaskToRun").Tag(transient.Tag).Err()
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
		return errors.Annotate(err, "failed to serialize the payload").Err()
	}

	constraints := make([]*remoteworkers.Constraint, len(task.Constraints))
	for i, ctr := range task.Constraints {
		constraints[i] = &remoteworkers.Constraint{
			Key:           "label:" + ctr.Key,
			AllowedValues: ctr.AllowedValues,
		}
	}

	var overallExpiration *timestamppb.Timestamp
	var queuingTimeout *durationpb.Duration
	var executionTimeout *durationpb.Duration

	if task.ExecutionTimeout == nil {
		// TODO(vadimsh): Get rid of this code path when it is no longer being hit.
		logging.Warningf(ctx, "No execution timeout set")
		overallExpiration = task.Expiry
	} else {
		// How much time left to sit in the queue.
		untilExpired := task.Expiry.AsTime().Sub(clock.Now(ctx))
		if untilExpired < 0 {
			untilExpired = 0
		}
		queuingTimeout = durationpb.New(untilExpired)
		// How much time there is to run once started.
		executionTimeout = task.ExecutionTimeout
		// The task must be done by that time.
		overallExpiration = timestamppb.New(
			task.Expiry.AsTime().Add(task.ExecutionTimeout.AsDuration()))
	}

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
		return errors.Reason("unexpected RBE gRPC status code in %s", reason).Err()
	}

	// Tell Swarming to switch to the next slice, if necessary.
	return s.expireSlice(ctx, task, reasonCode, reason.Error())
}

// ExpireSliceBasedOnReservation checks the reservation status by calling
// the Reservations API and invokes ExpireSlice if the reservation is dead.
//
// It is ultimately invoked from a PubSub push handler when handling
// notifications from the RBE scheduler.
//
// `reservationName` is a full reservation name, including the project and
// RBE instance IDs: `projects/.../instances/.../reservations/...`.
func (s *ReservationServer) ExpireSliceBasedOnReservation(ctx context.Context, reservationName string) error {
	// Get the up-to-date state of the reservation.
	reservation, err := s.rbe.GetReservation(ctx, &remoteworkers.GetReservationRequest{
		Name:        reservationName,
		WaitSeconds: 0,
	})
	err = grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
	if err != nil {
		return errors.Annotate(err, "failed to fetch reservation %s", reservationName).Err()
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
		return errors.Annotate(err, "failed to unmarshal reservation %s payload", reservationName).Err()
	}
	logging.Infof(ctx, "TaskPayload:\n%s", prettyProto(&payload))

	// TaskResult contains extra information supplied by the bot when it was
	// closing the lease. It is empty if the lease didn't reach the bot at all.
	if reservation.Result != nil {
		var result internalspb.TaskResult
		if err := reservation.Result.UnmarshalTo(&result); err != nil {
			return errors.Annotate(err, "failed to unmarshal reservation result").Err()
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
		return errors.Annotate(err, "bad TaskPayload").Err()
	}
	switch err := datastore.Get(ctx, ttr); {
	case err == datastore.ErrNoSuchEntity:
		logging.Warningf(ctx, "TaskToRun entity is already gone")
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to fetch TaskToRun").Tag(transient.Tag).Err()
	case !ttr.IsReapable():
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

	// Tell Swarming to switch to the next slice, if necessary
	logging.Warningf(ctx, "Expiring slice with %s: %s", reasonCode, statusErr)
	if err := s.expireSlice(ctx, &payload, reasonCode, statusErr.Error()); err != nil {
		return errors.Annotate(err, "failed to expire the slice").Tag(transient.Tag).Err()
	}
	return nil
}

// expireSlice calls Swarming Python's ExpireSlice RPC.
func (s *ReservationServer) expireSlice(ctx context.Context, task *internalspb.TaskPayload, code internalspb.ExpireSliceRequest_Reason, details string) error {
	_, err := s.internals.ExpireSlice(ctx, &internalspb.ExpireSliceRequest{
		TaskId:         task.TaskId,
		TaskToRunShard: task.TaskToRunShard,
		TaskToRunId:    task.TaskToRunId,
		Reason:         code,
		Details:        details,
	})
	return err
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

// EnqueueCancel enqueues a `rbe-cancel` task to cancel RBE reservation of
// a task.
func EnqueueCancel(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: &internalspb.CancelRBETask{
			RbeInstance:   tr.RBEInstance,
			ReservationId: ttr.RBEReservation,
			DebugInfo: &internalspb.CancelRBETask_DebugInfo{
				Created:  timestamppb.New(clock.Now(ctx)),
				TaskName: tr.Name,
			},
		},
	})
}
