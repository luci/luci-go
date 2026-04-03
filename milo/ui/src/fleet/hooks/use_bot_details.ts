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

import { useBot, useBotInfo } from '@/fleet/hooks/swarming_hooks';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

export const useBotDetails = (
  swarmingHost: string,
  dutId?: string,
  botId?: string,
) => {
  const effectiveHost = swarmingHost || SETTINGS.swarming.defaultHost;
  const client = useBotsClient(effectiveHost);

  const isEnabled = !!swarmingHost;

  const botDataFromDut = useBot(client, dutId || '', {
    enabled: isEnabled && !!dutId && !botId,
  });
  const botDataFromId = useBotInfo(client, botId || '', {
    enabled: isEnabled && !!botId,
  });
  const activeData = botId ? botDataFromId : botDataFromDut;

  const data = activeData.data;
  const botFound = botId ? !!data : botDataFromDut.botFound;
  const botState = JSON.parse(data?.state || '{}');
  const prettyState = JSON.stringify(botState, undefined, 2);

  return {
    ...activeData,
    botFound,
    botState,
    prettyState,
  };
};
