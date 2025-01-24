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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
)

// botCallInfo populates fields of BotEventCallInfo based on the state in the
// context.
func botCallInfo(ctx context.Context, info *model.BotEventCallInfo) *model.BotEventCallInfo {
	state := auth.GetState(ctx)
	info.ExternalIP = state.PeerIP().String()
	info.AuthenticatedAs = state.PeerIdentity()
	return info
}

// checkBotHealthInfo extract quarantine and maintenance status of the bot from
// the state and the "quarantined" dimension values, and (if the bot is indeed
// quarantined) updates the state dict to indicate that.
//
// Returns the extracted health info and the updated state dict.
func checkBotHealthInfo(state botstate.Dict, quarantinedDim []string) (model.BotHealthInfo, botstate.Dict, error) {
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

	healthInfo := model.BotHealthInfo{
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
