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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

// ReservationServer is responsible for creating and canceling RBE reservations.
type ReservationServer struct {
	rbe           remoteworkers.ReservationsClient
	serverVersion string

	// This is temporary until there's a Swarming client that can be mocked.
	testReservationDenied func(reason error) error
}

// NewReservationServer creates a new reservation server given an RBE client
// connection.
func NewReservationServer(ctx context.Context, cc grpc.ClientConnInterface, serverVersion string) *ReservationServer {
	return &ReservationServer{
		rbe:           remoteworkers.NewReservationsClient(cc),
		serverVersion: serverVersion,
	}
}

// RegisterTQTasks registers task queue handlers.
//
// Tasks are actually submitted from the Python side.
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
}

// handleEnqueueRBETask is responsible for creating a reservation.
func (s *ReservationServer) handleEnqueueRBETask(ctx context.Context, task *internalspb.EnqueueRBETask) (err error) {
	// TODO(vadimsh): Fetch TaskToRun and verify it is still pending. This is
	// important to eventually skip TQ tasks representing "broken" reservations
	// (e.g. misconfigured RBE instance or some internal errors). Swarming
	// eventually marks their TaskToRun as consumed.

	// TODO(vadimsh): The following edge case is broken:
	// 1. Try to submit a reservation, get FailedPrecondition, because no bots.
	// 2. Try to report this to Swarming in reservationDenied(...) and fail.
	// 3. This causes a retry of the whole thing.
	// 4. Retry submitting the reservation, getting AlreadyExists now.
	// 5. Call GetReservation to see if it is running or failed to be submitted.
	// 6. RBE returns "Remote Build Execution - internal server error" :-/

	// On fatal errors need to mark the TaskToRun in Swarming DB as failed. On
	// transient errors just let the TQ to call us again for a retry. Note that
	// CreateReservation will fail with AlreadyExists if we actually managed to
	// create the reservation on the previous attempt.
	defer func() {
		if err != nil && !transient.Tag.In(err) {
			// Something is fatally broken, report this to Swarming.
			if derr := s.reservationDenied(ctx, task, err); derr != nil {
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
				if status.Code(err) == codes.FailedPrecondition {
					err = tq.Ignore.Apply(err)
				} else {
					err = tq.Fatal.Apply(err)
				}
			}
		}
	}()

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

	logging.Infof(ctx, "Creating reservation %q", task.Payload.ReservationId)
	reservationName := fmt.Sprintf("%s/reservations/%s", task.RbeInstance, task.Payload.ReservationId)
	reservation, err := s.rbe.CreateReservation(ctx, &remoteworkers.CreateReservationRequest{
		Parent: task.RbeInstance,
		Reservation: &remoteworkers.Reservation{
			Name:           reservationName,
			Payload:        payload,
			ExpireTime:     task.Expiry,
			Priority:       task.Priority,
			Constraints:    constraints,
			RequestedBotId: task.RequestedBotId,
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
func (s *ReservationServer) reservationDenied(ctx context.Context, task *internalspb.EnqueueRBETask, reason error) error {
	// TODO(vadimsh): Implement.
	logging.Errorf(ctx, "Failed to submit RBE reservation %q: %s", task.Payload.ReservationId, reason)
	if s.testReservationDenied != nil {
		return s.testReservationDenied(reason)
	}
	return nil
}
