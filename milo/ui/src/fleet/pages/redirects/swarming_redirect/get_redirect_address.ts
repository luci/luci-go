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

import { To } from 'react-router-dom';

import { DecoratedClient } from '@/common/hooks/prpc_query';
import { BASE_DIMENSIONS } from '@/fleet/components/device_table/dimensions';
import {
  BotRequest,
  BotsClientImpl,
} from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

const prefix = '/ui/fleet/labs/';

/* Maps swarming path to fleet console paths.
 * Supports:
 *    https://chromeos-swarming.appspot.com/botlist
 *    https://chromeos-swarming.appspot.com/bot?id={BOT_ID}
 * */
export const getRedirectAddress = async (
  url: string | undefined,
  searchParams: URLSearchParams,
  swarmingClient: DecoratedClient<BotsClientImpl>,
): Promise<To> => {
  switch (url) {
    case 'botlist':
      return {
        pathname: prefix + 'devices',
        search: botListParseParams(searchParams),
      };
    case 'bot': {
      const bot_id = searchParams.get('id');
      if (!bot_id) throw Error(`Missing bot id`);

      const res = await swarmingClient.GetBot(
        BotRequest.fromPartial({
          botId: bot_id,
        }),
      );

      const dutName = res.dimensions.find(
        ({ key }) => key === 'dut_name',
      )?.value;
      if (!dutName) throw Error(`Cannot find dut_name of device ${bot_id}`);

      return {
        pathname: prefix + `devices/${dutName}`,
      };
    }
  }

  throw Error(`No page mapping found for page ${url}`);
};

const botListParseParams = (searchParams: URLSearchParams): string => {
  const out = new URLSearchParams([
    ...convertFilters(searchParams),
    ...convertColumns(searchParams),
    ...convertOrderBy(searchParams),
  ]);
  return '?' + out.toString();
};

const convertFilters = (searchParams: URLSearchParams) => {
  const filters = searchParams.getAll('f');
  if (filters.length === 0) return [];

  const filterObj: Record<string, string[]> = {};
  for (const f of filters) {
    let [key, val] = f.split(':', 2);
    val = `"${val}"`;

    if (!BASE_DIMENSIONS.some((bd) => bd.id === key)) key = 'labels.' + key;

    if (filterObj[key]) filterObj[key].push(val);
    else filterObj[key] = [val];
  }

  return [
    [
      'filters',
      Object.entries(filterObj)
        // Swarming uses ANDs and we only support ORs between fields
        .filter(([_, vals]) => vals.length === 1)
        .map(([key, vals]) => `${key} = ${vals[0]}`)
        .join(' '),
    ],
  ];
};

const convertColumns = (searchParams: URLSearchParams) => {
  const columns = searchParams.getAll('c');
  return columns.map((col) => ['c', col]);
};

const convertOrderBy = (searchParams: URLSearchParams) => {
  const sParam = searchParams.get('s');
  const ascDesc = searchParams.get('d');

  if (!sParam) return [];

  const by = !BASE_DIMENSIONS.some((bd) => bd.id === sParam)
    ? `labels.${sParam}`
    : sParam;

  if (ascDesc === 'desc') return [['order_by', `${by} desc`]];
  return [['order_by', by]];
};
