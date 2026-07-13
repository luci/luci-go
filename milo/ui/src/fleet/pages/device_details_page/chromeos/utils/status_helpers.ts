// Copyright 2026 The LUCI Authors.
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

import { colors, unknownStateColor } from '@/fleet/theme/colors';
import { Chameleon_AudioBoxJackPlugger } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/chromeos/lab/chameleon.pb';
import { State } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/state.pb';

export const getStatusColor = (
  state: number | string | null | undefined,
): string => {
  switch (state) {
    case 'STATE_SERVING':
    case State.STATE_SERVING:
    case 'STATE_DEPLOYED_TESTING':
    case State.STATE_DEPLOYED_TESTING:
      return colors.transparent;
    case 'STATE_NEEDS_REPAIR':
    case State.STATE_NEEDS_REPAIR:
    case 'STATE_NEEDS_RESET':
    case State.STATE_NEEDS_RESET:
    case 'STATE_REPAIR_FAILED':
    case State.STATE_REPAIR_FAILED:
      return colors.red[100];
    case 'STATE_REGISTERED':
    case State.STATE_REGISTERED:
    case 'STATE_DEPLOYING':
    case State.STATE_DEPLOYING:
    case 'STATE_DEPLOYED_PRE_SERVING':
    case State.STATE_DEPLOYED_PRE_SERVING:
      return colors.yellow[100];
    case 'STATE_DISABLED':
    case State.STATE_DISABLED:
    case 'STATE_DECOMMISSIONED':
    case State.STATE_DECOMMISSIONED:
      return colors.grey[300];
    case 'STATE_RESERVED':
    case State.STATE_RESERVED:
      return colors.purple[100];
    default:
      return unknownStateColor;
  }
};

export const getJackpluggerColor = (
  val: number | string | null | undefined,
): 'success' | 'error' | 'default' => {
  switch (val) {
    case 'AUDIOBOX_JACKPLUGGER_WORKING':
    case Chameleon_AudioBoxJackPlugger.AUDIOBOX_JACKPLUGGER_WORKING:
      return 'success';
    case 'AUDIOBOX_JACKPLUGGER_BROKEN':
    case Chameleon_AudioBoxJackPlugger.AUDIOBOX_JACKPLUGGER_BROKEN:
      return 'error';
    default:
      return 'default';
  }
};
