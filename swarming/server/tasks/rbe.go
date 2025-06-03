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

package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
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

// EnqueueRBECancel enqueues a `rbe-cancel` task to cancel RBE reservation of
// a task.
func EnqueueRBECancel(ctx context.Context, disp *tq.Dispatcher, tr *model.TaskRequest, ttr *model.TaskToRun) error {
	return disp.AddTask(ctx, &tq.Task{
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

// EnqueueRBENew enqueues a `rbe-enqueue` task to create RBE reservation for
// a task.
func EnqueueRBENew(ctx context.Context, disp *tq.Dispatcher, tr *model.TaskRequest, ttr *model.TaskToRun, cfg *cfg.Config) error {
	taskID := model.RequestKeyToTaskID(tr.Key, model.AsRequest)
	sliceIndex := ttr.TaskSliceIndex()
	s := tr.TaskSlices[sliceIndex]
	rbeEffectiveBotIDDim, err := getRBEEffectiveBotIDDimension(ctx, s, cfg, tr.Pool())
	if err != nil {
		return err
	}
	botID, constraints, err := dimsToBotIDAndConstraints(ctx, s.Properties.Dimensions, rbeEffectiveBotIDDim, tr.Pool())
	if err != nil {
		return err
	}

	// Add extra 30 seconds to compensate for bot's own overhead.
	timeout := s.Properties.ExecutionTimeoutSecs + s.Properties.GracePeriodSecs + 30

	logging.Infof(ctx, "RBE: enqueuing task to launch %s", ttr.RBEReservation)
	return disp.AddTask(ctx, &tq.Task{
		Title: fmt.Sprintf("%s-%d-%d", taskID, sliceIndex, ttr.RetryCount),
		Payload: &internalspb.EnqueueRBETask{
			Payload: &internalspb.TaskPayload{
				ReservationId:  ttr.RBEReservation,
				TaskId:         taskID,
				SliceIndex:     int32(sliceIndex),
				TaskToRunShard: ttr.MustShardIndex(),
				TaskToRunId:    ttr.Key.IntID(),
				DebugInfo: &internalspb.TaskPayload_DebugInfo{
					Created:  timestamppb.New(clock.Now(ctx)),
					TaskName: tr.Name,
				},
			},
			RbeInstance:         tr.RBEInstance,
			Expiry:              timestamppb.New(ttr.Expiration.Get()),
			RequestedBotId:      botID,
			Constraints:         constraints,
			Priority:            int32(tr.Priority),
			SchedulingAlgorithm: tr.SchedulingAlgorithm,
			ExecutionTimeout:    durationpb.New(time.Duration(timeout) * time.Second),
			WaitForCapacity:     s.WaitForCapacity,
		},
	})
}

func getRBEEffectiveBotIDDimension(ctx context.Context, s model.TaskSlice, cfg *cfg.Config, pool string) (string, error) {
	if pool != "" {
		poolCfg := cfg.Pool(pool)
		if poolCfg == nil {
			logging.Warningf(ctx, "Pool %q not found, assume it doesn't use effective bot ID dimension", pool)
			return "", nil
		}
		return poolCfg.RBEEffectiveBotIDDimension, nil
	}

	// The task doesn't use pool, it must use bot ID. Tasks that use bot ID are
	// validated to have at most one bot ID.
	botIDs := s.Properties.Dimensions["id"]
	if len(botIDs) != 1 || strings.Contains(botIDs[0], "|") {
		panic(fmt.Sprintf("expecting a task without pool dimension to have exactly one id dimension, got %q", botIDs))
	}

	// Lookup the pool configs for that particular bot.
	botID := botIDs[0]
	conf, err := cfg.RBEConfig(botID)
	if err != nil {
		return "", errors.Fmt("conflicting RBE config for bot %q: %s: %w", botID, err, ErrBadReservation)
	}
	return conf.EffectiveBotIDDimension, nil
}

func dimsToBotIDAndConstraints(ctx context.Context, dims model.TaskDimensions, rbeEffectiveBotIDDim, pool string) (botID string, constraints []*internalspb.EnqueueRBETask_Constraint, err error) {
	var effectiveBotIDFromBot, effectiveBotID string
	for key, values := range dims {
		switch key {
		case "id":
			if len(values) != 1 || strings.Contains(values[0], "|") {
				// This has been validated when the task was submitted.
				panic(fmt.Sprintf("invalid id dimension: %q", values))
			}
			botID = values[0]
			if rbeEffectiveBotIDDim != "" {
				effectiveBotIDFromBot, err = fetchEffectiveBotID(ctx, botID)
				if err != nil {
					return "", nil, err
				}
			}
		case rbeEffectiveBotIDDim:
			if len(values) != 1 || strings.Contains(values[0], "|") {
				// Normally the task is validated so that the effective bot ID dimension
				// has at most one value, but here we may be retrying the submission and
				// may be using a different version of the config (not the one used
				// during the initial validation). A different rbeEffectiveBotIDDim may
				// be "broken". This should be rare. Return an error to give up on this
				// reservation.
				return "", nil, errors.Fmt("invalid effective bot ID dimension %q: %q: %w", rbeEffectiveBotIDDim, values, ErrBadReservation)
			}
			if pool != "" {
				effectiveBotID = model.RBEEffectiveBotID(pool, rbeEffectiveBotIDDim, values[0])
			}
		default:
			for _, v := range values {
				constraints = append(constraints, &internalspb.EnqueueRBETask_Constraint{
					Key:           key,
					AllowedValues: strings.Split(v, "|"),
				})
			}
		}
	}
	if effectiveBotIDFromBot != "" && effectiveBotID != "" && effectiveBotIDFromBot != effectiveBotID {
		return "", nil, errors.Fmt("conflicting effective bot IDs: %q (according to bot %q) and %q (according to task): %w",
			effectiveBotIDFromBot, botID, effectiveBotID, ErrBadReservation)

	}
	if effectiveBotIDFromBot != "" {
		botID = effectiveBotIDFromBot
	}
	if effectiveBotID != "" {
		botID = effectiveBotID
	}

	return botID, constraints, nil
}

func fetchEffectiveBotID(ctx context.Context, botID string) (string, error) {
	info := &model.BotInfo{Key: model.BotInfoKey(ctx, botID)}
	if err := datastore.Get(datastore.WithoutTransaction(ctx), info); err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			logging.Errorf(ctx, "Failed to get effective bot ID for %q: %s", botID, err)
			return "", nil
		}
		return "", errors.Fmt("failed to get effective bot ID for %q: %w", botID, err)
	}

	if info.RBEEffectiveBotID == "" {
		// If cannot find the effective Bot ID for the bot,
		// it could be because
		// * the bot is still in handshake;
		// * the effective_bot_id_dimension is newly added and the bot has not
		//   call bot/poll yet;
		// * the bot doesn't provide valid effective Bot ID dimension (missing
		//   or multiple);
		// * the bot belongs to multiple pools to make the effective Bot ID
		//   feature not usable;
		// Add a log here and move on, the task will either be rejected by RBE
		// with NO_RESOURCE or execute with botID.
		//
		// TODO: Such tasks may get stuck for a long time if they have
		// WaitForCapacity set.
		logging.Debugf(ctx, "Effective bot ID for %q not found", botID)
	}
	return info.RBEEffectiveBotID, nil
}
