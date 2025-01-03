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

import { render, screen } from '@testing-library/react';

import { BOT_STATUS_LABEL_MAP, BotStatus } from '@/swarming/tools/bot_status';

import { BotStatusTable } from './bot_status_table';

describe('BotStatusTable', () => {
  test('should display correctly when there are some bots', () => {
    render(
      <BotStatusTable
        stats={{
          [BotStatus.Idle]: 2,
          [BotStatus.Busy]: 4,
          [BotStatus.Quarantined]: 1,
          [BotStatus.Dead]: 3,
          // Delete bots have been filtered out. Declare it regardless to pass
          // type checking.
          [BotStatus.Deleted]: 0,
        }}
        totalBots={10}
      />,
    );

    let statusRow: HTMLElement;
    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Idle],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('2');
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '20%',
    );

    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Busy],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('4');
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '40%',
    );

    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Quarantined],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('1');
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '10%',
    );

    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Dead],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('3');
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '30%',
    );
  });

  test("should display correctly when there're no bots", () => {
    render(
      <BotStatusTable
        stats={{
          [BotStatus.Idle]: 0,
          [BotStatus.Busy]: 0,
          [BotStatus.Quarantined]: 0,
          [BotStatus.Dead]: 0,
          // Delete bots have been filtered out. Declare it regardless to pass
          // type checking.
          [BotStatus.Deleted]: 0,
        }}
        totalBots={0}
      />,
    );

    let statusRow: HTMLElement;
    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Idle],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('0');
    // Width should be 0% rather than NaN%.
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '0%',
    );

    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Busy],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('0');
    // Width should be 0% rather than NaN%.
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '0%',
    );

    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Quarantined],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('0');
    // Width should be 0% rather than NaN%.
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '0%',
    );

    statusRow = screen.getByText(
      BOT_STATUS_LABEL_MAP[BotStatus.Dead],
    ).parentElement!;
    expect(statusRow.querySelector('td:nth-child(2)')).toHaveTextContent('0');
    // Width should be 0% rather than NaN%.
    expect(statusRow.querySelector('td:nth-child(3)>div')).toHaveStyleRule(
      'width',
      '0%',
    );
  });
});
