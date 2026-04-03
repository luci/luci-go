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
import { BROWSER_SWARMING_SOURCE } from '@/fleet/constants/browser';
import { getDutName } from '@/fleet/utils/swarming';
import { BotsClientImpl } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

const defaultPrefix = '/ui/fleet/labs/p/chromeos/';

export const getRedirectAddress = async (
  url: string | undefined,
  searchParams: URLSearchParams,
  swarmingClient: DecoratedClient<BotsClientImpl>,
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

      const dutName = await getDutName(swarmingClient, bot_id);
      if (!dutName) throw Error(`Cannot find dut_name of device ${bot_id}`);

      return {
        pathname: targetPrefix + `devices/${dutName}`,
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
    let [key, val] = f.split(':', 2);
    val = `"${val}"`;

    if (isBrowser) {
      key = `${BROWSER_SWARMING_SOURCE}."${key}"`;
    } else if (!baseDimensions.includes(key)) {
      key = 'labels.' + key;
    }

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
