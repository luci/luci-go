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

import { Link } from '@mui/material';
import { DateTime } from 'luxon';

import { Timestamp } from '@/common/components/timestamp';
import { NUMERIC_TIME_FORMAT } from '@/common/tools/time_utils';
import { BotInfo } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import {
  BOT_STATUS_COLOR_MAP,
  BOT_STATUS_LABEL_MAP,
  getBotStatus,
} from '@/swarming/tools/bot_status';
import { getBotUrl } from '@/swarming/tools/utils';

interface BotRowProps {
  readonly swarmingHost: string;
  readonly bot: BotInfo;
}

function BotRow({ swarmingHost, bot }: BotRowProps) {
  const status = getBotStatus(bot);

  return (
    <tr
      css={{
        '& td': {
          padding: 5,
          textAlign: 'center',
        },
      }}
    >
      <td>
        <Link href={getBotUrl(swarmingHost, bot.botId)}>{bot.botId}</Link>
      </td>
      <td css={{ backgroundColor: BOT_STATUS_COLOR_MAP[status] }}>
        {BOT_STATUS_LABEL_MAP[status]}
      </td>
      <td>
        <Timestamp
          datetime={DateTime.fromISO(bot.lastSeenTs!)}
          format={NUMERIC_TIME_FORMAT}
        />
      </td>
    </tr>
  );
}

export interface BotTableProps {
  readonly swarmingHost: string;
  readonly bots: readonly BotInfo[];
}

export function BotTable({ swarmingHost, bots }: BotTableProps) {
  return (
    <table
      css={{
        '& tr:nth-of-type(odd)': {
          backgroundColor: 'var(--block-background-color)',
        },
      }}
    >
      <thead>
        <th>Name</th>
        <th>Status</th>
        <th>Last Seen</th>
      </thead>
      <tbody>
        {bots.map((bot) => (
          <BotRow key={bot.botId} swarmingHost={swarmingHost} bot={bot} />
        ))}
      </tbody>
    </table>
  );
}
