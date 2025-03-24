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

import { DecoratedClient } from '@/common/hooks/prpc_query';
import {
  BotRequest,
  BotsClientImpl,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

export const getDutName = async (
  swarmingClient: DecoratedClient<BotsClientImpl>,
  botId: string,
) => {
  if (!botId) throw Error(`Missing bot id`);

  const res = await swarmingClient.GetBot(
    BotRequest.fromPartial({
      botId: botId,
    }),
  );

  return res.dimensions.find(({ key }) => key === 'dut_name')?.value?.at(0);
};
