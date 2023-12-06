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

// Command fakesubmit submits an RBE reservation for testing fakebot.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
)

var (
	pool         = flag.String("pool", "local-test", "Value for `pool` dimension")
	rbeInstance  = flag.String("rbe-instance", "projects/chromium-swarm-dev/instances/default_instance", "Full RBE instance name to use for tests")
	expiration   = flag.Duration("expiration", 10*time.Minute, "Task expiration time")
	tasks        = flag.Int("tasks", 1, "How many tasks to submit in parallel")
	noop         = flag.Bool("noop", false, "If set, mark tasks as noop")
	taskIDPrefix = flag.String("task-id-prefix", "", "Prefix for reservation IDs")
)

func main() {
	flag.Parse()
	ctx := gologger.StdConfig.Use(context.Background())
	if err := run(ctx); err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	creds, err := auth.NewAuthenticator(ctx, auth.SilentLogin, chromeinfra.SetDefaultAuthOptions(auth.Options{
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
		},
	})).PerRPCCredentials()
	if err != nil {
		return err
	}

	cc, err := grpc.DialContext(ctx, "remotebuildexecution.googleapis.com:443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
		grpc.WithBlock(),
	)
	if err != nil {
		return err
	}
	rbe := remoteworkers.NewReservationsClient(cc)

	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	signals.HandleInterrupt(func() {
		logging.Infof(ctx, "Got termination signal")
		cancel()
	})

	if *taskIDPrefix == "" {
		*taskIDPrefix = uuid.New().String()
	}

	type execResult struct {
		taskID      string
		start       time.Time
		end         time.Time
		reservation *remoteworkers.Reservation
		err         error
	}
	results := make(chan execResult, *tasks)

	for i := 0; i < *tasks; i++ {
		i := i
		go func() {
			taskID := fmt.Sprintf("%s-%04d", *taskIDPrefix, i)
			start := clock.Now(ctx)
			reservation, err := execTask(ctx, loopCtx, rbe, taskID)
			results <- execResult{
				taskID:      taskID,
				start:       start,
				end:         clock.Now(ctx),
				reservation: reservation,
				err:         err,
			}
		}()
	}

	for i := 0; i < *tasks; i++ {
		res := <-results
		switch {
		case res.err != nil:
			logging.Errorf(ctx, "%s: %s", res.taskID, res.err)
		case res.reservation == nil:
			logging.Warningf(ctx, "%s: canceled", res.taskID)
		default:
			reservation := res.reservation
			if reservation.Status.GetCode() != 0 {
				logging.Infof(ctx, "%s: %s %s", res.taskID, reservation.AssignedBotId, status.FromProto(reservation.Status))
			} else {
				logging.Infof(ctx, "%s: %s", res.taskID, reservation.AssignedBotId)
			}
		}
	}

	return nil
}

func execTask(ctx, loopCtx context.Context, rbe remoteworkers.ReservationsClient, taskID string) (*remoteworkers.Reservation, error) {
	reservationName := fmt.Sprintf("%s/reservations/%s", *rbeInstance, taskID)

	payload, err := anypb.New(&internalspb.TaskPayload{
		TaskId: taskID,
		Noop:   *noop,
	})
	if err != nil {
		return nil, err
	}

	reservation, err := rbe.CreateReservation(ctx, &remoteworkers.CreateReservationRequest{
		Parent: *rbeInstance,
		Reservation: &remoteworkers.Reservation{
			Name:       reservationName,
			Payload:    payload,
			ExpireTime: timestamppb.New(time.Now().Add(*expiration)),
			Constraints: []*remoteworkers.Constraint{
				{Key: "label:pool", AllowedValues: []string{*pool}},
			},
		},
	})
	if status.Code(err) == codes.AlreadyExists {
		reservation, err = rbe.GetReservation(ctx, &remoteworkers.GetReservationRequest{
			Name: reservationName,
		})
		if err != nil {
			return nil, errors.Annotate(err, "GetReservation").Err()
		}
	} else if err != nil {
		return nil, errors.Annotate(err, "CreateReservation").Err()
	}

	for loopCtx.Err() == nil {
		if reservation.State == remoteworkers.ReservationState_RESERVATION_COMPLETED || reservation.State == remoteworkers.ReservationState_RESERVATION_CANCELLED {
			return reservation, nil
		}
		reservation, err = rbe.GetReservation(loopCtx, &remoteworkers.GetReservationRequest{
			Name:        reservationName,
			WaitSeconds: 60,
		})
		if err != nil {
			if loopCtx.Err() != nil {
				break
			}
			return nil, err
		}
	}

	_, err = rbe.CancelReservation(ctx, &remoteworkers.CancelReservationRequest{
		Name:   reservationName,
		Intent: remoteworkers.CancelReservationIntent_ANY,
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}
