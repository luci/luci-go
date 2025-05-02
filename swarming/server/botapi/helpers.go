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

package botapi

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/validate"
)

var wrongTaskIDErr = errors.New("the bot is not associated with this task on the server")

// peekBotID extracts bot ID from the dimensions.
//
// It is called early when authorizing bots (in particular ancient ones that
// don't use session tokens). Doesn't do full validation, validates only the
// bot ID.
//
// Returns gRPC errors.
func peekBotID(dims map[string][]string) (string, error) {
	idDim := dims["id"]
	if len(idDim) == 0 {
		return "", status.Errorf(codes.InvalidArgument, "no `id` dimension reported")
	}
	botID := idDim[0]
	if err := validate.DimensionValue("id", botID); err != nil {
		return "", status.Errorf(codes.InvalidArgument, "bad bot `id` %q: %s", botID, err)
	}
	return botID, nil
}

// botCallInfo populates fields of botinfo.CallInfo based on the state in the
// context.
func botCallInfo(ctx context.Context, info *botinfo.CallInfo) *botinfo.CallInfo {
	state := auth.GetState(ctx)
	info.ExternalIP = state.PeerIP().String()
	info.AuthenticatedAs = state.PeerIdentity()
	return info
}

// updateBotHealthInfo derives quarantine and maintenance status of the bot
// based on the state dict, the "quarantined" dimension values and the given
// list of request validation errors.
//
// If the bot is indeed quarantined or there are errors present, updates the
// state dict to indicate that.
//
// Returns the extracted health info and the updated state dict.
func updateBotHealthInfo(state botstate.Dict, quarantinedDim []string, errs errors.MultiError) (botinfo.HealthInfo, botstate.Dict, error) {
	// First read the quarantine message from the state.
	var msg string
	var val any
	if err := state.Read(botstate.QuarantinedKey, &val); err != nil {
		msg = fmt.Sprintf("Can't read self-quarantine message from state: %s", err)
	} else {
		msg = model.QuarantineMessage(val)
	}

	// If it is not there or trivial, try to find it in dimensions instead.
	if msg == "" || msg == model.GenericQuarantineMessage {
		if len(quarantinedDim) > 0 {
			switch strings.ToLower(quarantinedDim[0]) {
			case "true", "1":
				msg = model.GenericQuarantineMessage
			case "":
				// Do nothing.
			default:
				msg = quarantinedDim[0]
			}
		}
	}

	// Append all validation error lines, if any.
	lines := make([]string, 0, len(errs)+1)
	if msg != "" {
		lines = append(lines, msg)
	}
	for _, err := range errs {
		lines = append(lines, err.Error())
	}
	msg = strings.Join(lines, "\n")

	healthInfo := botinfo.HealthInfo{
		Quarantined: msg,
		Maintenance: state.MustReadString(botstate.MaintenanceKey),
	}

	// Put the quarantine message into the state as well to have it there
	// consistently, regardless of how it was reported by the bot initially.
	// Only replace a missing or trivial message. Do not overwrite any existing
	// message (it can theoretically be more informative).
	var err error
	if healthInfo.Quarantined != "" {
		state, err = botstate.Edit(state, func(state *botstate.EditableDict) error {
			var cur any
			if err := state.Read(botstate.QuarantinedKey, &cur); err != nil {
				return errors.Annotate(err, "reading %s", botstate.QuarantinedKey).Err()
			}
			if msg := model.QuarantineMessage(cur); msg == "" || msg == model.GenericQuarantineMessage {
				if err := state.Write(botstate.QuarantinedKey, healthInfo.Quarantined); err != nil {
					return errors.Annotate(err, "writing %s", botstate.QuarantinedKey).Err()
				}
			}
			return nil
		})
	}
	return healthInfo, state, err
}

// validateTaskID validates the provided taskID.
//
// It checks
// * The correctness of the taskID;
// * If it's the current task ID the bot is associated with;
// * Whether there's a TaskRequest entity saved for the taskID.
func validateTaskID(ctx context.Context, taskID, botTaskID string) (*model.TaskRequest, error) {
	if taskID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task ID is required")
	}
	if botTaskID != taskID {
		return nil, wrongTaskIDErr
	}
	reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
	if err != nil {
		// This should never happen, the task ID is the same as the one from
		// datastore, which should have been validated.
		return nil, status.Errorf(codes.FailedPrecondition, "invalid task ID %q", taskID)
	}
	tr, err := model.FetchTaskRequest(ctx, reqKey)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "task %q not found", taskID)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "failed to get task %q: %s", taskID, err)
	}
	return tr, nil
}
