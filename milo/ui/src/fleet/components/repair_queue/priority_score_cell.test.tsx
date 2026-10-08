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

import { fireEvent, render, screen } from '@testing-library/react';

import { RepairPriorityRule } from './editable_rule_row';
import { PriorityScoreCell } from './priority_score_cell';

describe('<PriorityScoreCell />', () => {
  const rulesById: ReadonlyMap<string, RepairPriorityRule> = new Map([
    [
      'rule-1',
      { id: 'rule-1', expressionAip160: 'model = "volteer"', weight: '300' },
    ],
    [
      'rule-2',
      { id: 'rule-2', expressionAip160: 'pool = "QUOTA"', weight: '350' },
    ],
  ]);

  it('itemizes matched rules and the total in the tooltip', async () => {
    render(
      <PriorityScoreCell
        item={{ priorityScore: '650', matchedRuleIds: ['rule-1', 'rule-2'] }}
        rulesById={rulesById}
      />,
    );

    fireEvent.mouseOver(screen.getByText('+650 pts'));

    expect(await screen.findByText('model = "volteer"')).toBeInTheDocument();
    expect(screen.getByText('+300 pts')).toBeInTheDocument();
    expect(screen.getByText('pool = "QUOTA"')).toBeInTheDocument();
    expect(screen.getByText('+350 pts')).toBeInTheDocument();
    expect(screen.getByText('= 650 pts')).toBeInTheDocument();
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
});
