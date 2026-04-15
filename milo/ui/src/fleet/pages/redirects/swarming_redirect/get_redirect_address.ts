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

import { To } from 'react-router';

import { DecoratedClient } from '@/common/hooks/prpc_query';
import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { BROWSER_SWARMING_SOURCE } from '@/fleet/constants/browser';
import { getDutName } from '@/fleet/utils/swarming';
import {
  FleetConsoleClientImpl,
  ListBrowserDevicesRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { BotsClientImpl } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

const defaultPrefix = '/ui/fleet/labs/p/chromeos/';

export const getRedirectAddress = async (
  url: string | undefined,
  searchParams: URLSearchParams,
  swarmingClient: DecoratedClient<BotsClientImpl>,
  fleetConsoleClient: DecoratedClient<FleetConsoleClientImpl>,
  baseDimensions: string[],
  platform: string = 'chromeos',
): Promise<To> => {
  const isBrowser = platform === 'chromium';
  const targetPrefix = isBrowser ? '/ui/fleet/p/chromium/' : defaultPrefix;

  switch (url) {
    case 'botlist':
      return {
        pathname: targetPrefix + 'devices',
        search: botListParseParams(searchParams, baseDimensions, isBrowser),
      };
    case 'bot': {
      const bot_id = searchParams.get('id');
      if (!bot_id) throw Error(`Missing bot id`);

      // Validate bot_id to prevent injection in the filter string.
      if (!/^[a-zA-Z0-9_-]+$/.test(bot_id)) {
        throw Error(`Invalid bot id: ${bot_id}`);
      }

      let identifier: string | undefined;
      // Browser devices are managed in UFS/Fleet Console and use different
      // naming conventions than ChromeOS DUTs. We look up the machine ID
      // by bot_id.
      if (isBrowser) {
        const res = await fleetConsoleClient.ListBrowserDevices(
          ListBrowserDevicesRequest.fromPartial({
            filter: `ufs."hostname" = "${bot_id}"`,
            pageSize: 1,
          }),
        );
        const device = res.devices[0];
        if (!device) {
          throw Error(
            `Cannot find device with bot_id ${bot_id} in Fleet Console`,
          );
        }
        identifier = device.id;
      } else {
        identifier = await getDutName(swarmingClient, bot_id);
        if (!identifier) {
          throw Error(`Cannot find dut_name of device ${bot_id}`);
        }
      }

      return {
        pathname: targetPrefix + `devices/${identifier}`,
      };
    }
  }

  throw Error(`No page mapping found for page ${url}`);
};

const botListParseParams = (
  searchParams: URLSearchParams,
  baseDimensions: string[],
  isBrowser: boolean,
): string => {
  const out = new URLSearchParams([
    ...convertFilters(searchParams, baseDimensions, isBrowser),
    ...convertColumns(searchParams),
    ...convertOrderBy(searchParams, baseDimensions, isBrowser),
  ]);
  return '?' + out.toString();
};

const convertFilters = (
  searchParams: URLSearchParams,
  baseDimensions: string[],
  isBrowser: boolean,
) => {
  const filters = searchParams.getAll('f');
  if (filters.length === 0) return [];

  const filterObj: Record<string, string[]> = {};
  for (const f of filters) {
    const parts = f.split(':', 2);
    let key = parts[0];
    const val = parts[1];
    if (val === undefined) {
      continue;
    }
    const vals = val.split('|');

    if (isBrowser) {
      key = `${BROWSER_SWARMING_SOURCE}."${key}"`;
    } else if (!baseDimensions.includes(key)) {
      key = 'labels.' + key;
    }

    if (!filterObj[key]) {
      filterObj[key] = [];
    }
    filterObj[key].push(...vals);
  }

  return [['filters', stringifyFilters(filterObj)]];
};

const convertColumns = (searchParams: URLSearchParams) => {
  const columns = searchParams.getAll('c');
  return columns.map((col) => ['c', col]);
};

const convertOrderBy = (
  searchParams: URLSearchParams,
  baseDimensions: string[],
  isBrowser: boolean,
) => {
  const sParam = searchParams.get('s');
  const ascDesc = searchParams.get('d');

  if (!sParam) return [];

  let by = sParam;
  if (isBrowser) {
    by = `${BROWSER_SWARMING_SOURCE}.${sParam}`;
  } else if (!baseDimensions.includes(sParam)) {
    by = `labels.${sParam}`;
  }

  if (ascDesc === 'desc') return [['order_by', `${by} desc`]];
  return [['order_by', by]];
};
