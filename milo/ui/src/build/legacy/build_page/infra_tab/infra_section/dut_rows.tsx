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

import { Link } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { OutputBuildInfra_Backend } from '@/build/types';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { getDutName } from '@/fleet/utils/swarming';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

export interface DutRowsProps {
  readonly backend: OutputBuildInfra_Backend;
}

export function DutRows({ backend }: DutRowsProps) {
  const swarmingClient = useBotsClient(DEVICE_TASKS_SWARMING_HOST);

  const botId = backend.task.id.target.startsWith('swarming://')
    ? backend.task.details?.bot_dimensions?.id?.[0]
    : undefined;
  const dutNameQuery = useQuery({
    queryKey: [swarmingClient, botId],
    queryFn: async () => {
      const dutName = await getDutName(swarmingClient, botId);
      if (dutName === undefined) throw new Error('Missing dut name');

      return dutName;
    },
    enabled: botId !== undefined,
  });

  if (!dutNameQuery.isSuccess) return null;

  return (
    <tr>
      <td>DUT running task:</td>
      <td>
        <Link
          href={`/ui/fleet/labs/devices/${dutNameQuery.data}`}
          target="_blank"
          rel="noopenner"
        >
          {dutNameQuery.data}
        </Link>
      </td>
    </tr>
  );
}
