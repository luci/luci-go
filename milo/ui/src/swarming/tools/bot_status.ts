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

import { BotInfo } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

export enum BotStatus {
  Idle,
  Busy,
  Quarantined,
  Dead,
  Deleted,
}

/**
 * Computes bot status.
 */
export function getBotStatus(bot: BotInfo): BotStatus {
  if (bot.deleted) {
    return BotStatus.Deleted;
  }
  if (bot.isDead) {
    return BotStatus.Dead;
  }
  if (bot.quarantined) {
    return BotStatus.Dead;
  }
  if (bot.maintenanceMsg || bot.taskId) {
    return BotStatus.Busy;
  }
  return BotStatus.Idle;
}

export const BOT_STATUS_LABEL_MAP = Object.freeze({
  [BotStatus.Idle]: 'Idle',
  [BotStatus.Busy]: 'Busy',
  [BotStatus.Quarantined]: 'Quarantined',
  [BotStatus.Dead]: 'Offline',
  [BotStatus.Deleted]: 'Deleted',
});

export const BOT_STATUS_COLOR_MAP = Object.freeze({
  [BotStatus.Idle]: 'var(--success-color)',
  [BotStatus.Busy]: 'var(--warning-color)',
  [BotStatus.Quarantined]: 'var(--exonerated-color)',
  [BotStatus.Dead]: 'var(--failure-color)',
  [BotStatus.Deleted]: 'var(--critical-failure-color)',
});
