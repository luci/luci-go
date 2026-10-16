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

import { fireEvent, render, screen, within } from '@testing-library/react';

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';

import { PriorityScoreCell } from './priority_score_cell';
import { parsePriorityRules } from './utils';

describe('<PriorityScoreCell />', () => {
  const filterBuilders = {
    model: new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([{ label: 'volteer', value: 'volteer' }]),
    pools: new StringListFilterCategoryBuilder().setLabel('Pool').setOptions([
      { label: 'QUOTA', value: 'QUOTA' },
      { label: 'DUT_POOL_QUOTA', value: 'DUT_POOL_QUOTA' },
    ]),
    dut_state: new StringListFilterCategoryBuilder()
      .setLabel('State')
      .setOptions([
        { label: 'Needs repair', value: 'needs_repair' },
        { label: 'Ready', value: 'ready' },
      ]),
    pool_health_pct: new RangeFilterCategoryBuilder()
      .setLabel('Pool Health')
      .setMin(0)
      .setMax(100)
      .setScaleFactor(0.01),
  };

  const rawRules = [
    { id: 'rule-1', expressionAip160: 'model = "volteer"', weight: '300' },
    { id: 'rule-2', expressionAip160: 'pools = "QUOTA"', weight: '350' },
  ];

  const rulesById = parsePriorityRules(rawRules, filterBuilders);

  it('itemizes matched rules and the total in the tooltip', async () => {
    render(
      <PriorityScoreCell
        item={{ priorityScore: '650', matchedRuleIds: ['rule-1', 'rule-2'] }}
        rulesById={rulesById}
      />,
    );

    fireEvent.mouseOver(screen.getByText('+650 pts'));

    expect(
      await screen.findByText('Priority score breakdown'),
    ).toBeInTheDocument();
    expect(screen.getByText('2 rules matched')).toBeInTheDocument();
    expect(screen.getByText('1 | Model IN volteer')).toBeInTheDocument();
    expect(screen.getByText('+300 pts')).toBeInTheDocument();
    expect(screen.getByText('1 | Pool IN QUOTA')).toBeInTheDocument();
    expect(screen.getByText('+350 pts')).toBeInTheDocument();
    expect(screen.getByText('Total score')).toBeInTheDocument();
    expect(screen.getByText('= 650 pts')).toBeInTheDocument();
  });

  it('renders compound rules and negative weight penalties in tooltip', async () => {
    const customRules = parsePriorityRules(
      [
        {
          id: 'rule-compound',
          expressionAip160: 'model = "volteer" AND pools = "DUT_POOL_QUOTA"',
          weight: '-50',
        },
      ],
      filterBuilders,
    );

    render(
      <PriorityScoreCell
        item={{ priorityScore: '-50', matchedRuleIds: ['rule-compound'] }}
        rulesById={customRules}
      />,
    );

    fireEvent.mouseOver(screen.getByText('-50 pts'));

    expect(
      await screen.findByText('Priority score breakdown'),
    ).toBeInTheDocument();
    expect(screen.getByText('1 rule matched')).toBeInTheDocument();
    const ruleRow = screen.getByTestId('matched-rule-rule-compound');
    expect(
      within(ruleRow).getByText('1 | Model IN volteer'),
    ).toBeInTheDocument();
    expect(
      within(ruleRow).getByText('1 | Pool IN DUT_POOL_QUOTA'),
    ).toBeInTheDocument();
    expect(within(ruleRow).getByText('-50 pts')).toBeInTheDocument();
    expect(screen.getByText('= -50 pts')).toBeInTheDocument();
  });

  it('falls back to the rule ID when a matched rule is unknown', async () => {
    render(
      <PriorityScoreCell
        item={{ priorityScore: '10', matchedRuleIds: ['rule-9'] }}
        rulesById={rulesById}
      />,
    );

    fireEvent.mouseOver(screen.getByText('+10 pts'));

    expect(await screen.findByText('Rule #rule-9')).toBeInTheDocument();
  });

  it('explains when no rules matched', async () => {
    render(
      <PriorityScoreCell
        item={{ priorityScore: '', matchedRuleIds: [] }}
        rulesById={rulesById}
      />,
    );

    fireEvent.mouseOver(screen.getByText('0 pts'));

    expect(
      await screen.findByText('No matched priority rules (0 pts)'),
    ).toBeInTheDocument();
  });

  it('renders native filter chips with label mappings from parsePriorityRules', async () => {
    const rules = parsePriorityRules(
      [
        {
          id: 'rule-complex',
          expressionAip160:
            'dut_state = "needs_repair" AND pool_health_pct <= 0.8',
          weight: '400',
        },
      ],
      filterBuilders,
    );

    render(
      <PriorityScoreCell
        item={{ priorityScore: '400', matchedRuleIds: ['rule-complex'] }}
        rulesById={rules}
      />,
    );

    fireEvent.mouseOver(screen.getByText('+400 pts'));

    expect(
      await screen.findByText('Priority score breakdown'),
    ).toBeInTheDocument();
    const ruleRow = screen.getByTestId('matched-rule-rule-complex');
    expect(
      within(ruleRow).getByText('1 | State IN Needs repair'),
    ).toBeInTheDocument();
    expect(within(ruleRow).getByText('Pool Health <= 80')).toBeInTheDocument();
    expect(within(ruleRow).getByText('+400 pts')).toBeInTheDocument();
  });
});
