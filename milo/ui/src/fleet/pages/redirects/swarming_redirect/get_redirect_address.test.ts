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
import { BotsClientImpl } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';

import { getRedirectAddress } from './get_redirect_address';

const prefix = '/ui/fleet/labs/';

describe('getRedirectAddress', () => {
  const swarmingClient = jest.mocked({
    GetBot: () => ({
      dimensions: [{ key: 'dut_name', value: 'dut_name_value' }],
    }),
  }) as unknown as DecoratedClient<BotsClientImpl>;

  describe('/botlist page', () => {
    test('basic botist', async () => {
      const to = await getRedirectAddress(
        'botlist',
        new URLSearchParams(),
        swarmingClient,
      );
      expect(to).toEqual({
        pathname: prefix + 'devices',
        search: '?',
      });
    });
    test('botlist filters', async () => {
      const to = await getRedirectAddress(
        'botlist',
        new URLSearchParams([
          ['f', 'dut_state:ready'],
          ['f', 'dut_state:needs_replacement'],
          ['f', 'dut_state:needs_repair'],
          ['f', 'label-bluetooth:True'],
        ]),
        swarmingClient,
      );
      expect(to).toEqual({
        search:
          '?' +
          new URLSearchParams([
            ['filters', `labels.label-bluetooth = "True"`],
          ]).toString(),
        pathname: prefix + 'devices',
      });
    });
    test('botlist columns', async () => {
      const to = await getRedirectAddress(
        'botlist',
        new URLSearchParams([
          ['c', 'col1'],
          ['c', 'col2'],
          ['c', 'col3'],
          ['c', 'col4'],
        ]),
        swarmingClient,
      );
      expect(to).toEqual({
        search:
          '?' +
          new URLSearchParams([
            ['c', 'col1'],
            ['c', 'col2'],
            ['c', 'col3'],
            ['c', 'col4'],
          ]).toString(),
        pathname: prefix + 'devices',
      });
    });
    test('botlist orderBy', async () => {
      const to = await getRedirectAddress(
        'botlist',
        new URLSearchParams([
          ['s', 'col1'],
          ['d', 'asc'],
        ]),
        swarmingClient,
      );
      expect(to).toEqual({
        search:
          '?' + new URLSearchParams([['order_by', 'labels.col1']]).toString(),
        pathname: prefix + 'devices',
      });
    });
    test('botlist orderBy desc', async () => {
      const to = await getRedirectAddress(
        'botlist',
        new URLSearchParams([
          ['s', 'col1'],
          ['d', 'desc'],
        ]),
        swarmingClient,
      );
      expect(to).toEqual({
        search:
          '?' +
          new URLSearchParams([['order_by', 'labels.col1 desc']]).toString(),
        pathname: prefix + 'devices',
      });
    });
    test('botlist orderBy no order', async () => {
      const to = await getRedirectAddress(
        'botlist',
        new URLSearchParams([['s', 'col1']]),
        swarmingClient,
      );
      expect(to).toEqual({
        search:
          '?' + new URLSearchParams([['order_by', 'labels.col1']]).toString(),
        pathname: prefix + 'devices',
      });
    });
  });

  describe('/bot page', () => {
    test('bot', async () => {
      const to = await getRedirectAddress(
        'bot',
        new URLSearchParams({ id: 'bot-id' }),
        swarmingClient,
      );
      expect(to).toEqual({
        pathname: prefix + 'devices/dut_name_value',
      });
    });
    test('bot missing id', async () => {
      await expect(() =>
        getRedirectAddress('bot', new URLSearchParams(), swarmingClient),
      ).rejects.toThrow('Missing bot id');
    });
  });
});
