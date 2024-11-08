// Copyright 2022 The LUCI Authors.
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

import '@testing-library/jest-dom';
import 'node-fetch';

import { screen } from '@testing-library/react';

import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';

import RuleDefinition from './rule_definition';

describe('Test RuleInfo component', () => {
  it('given a rule, then should display rule details', async () => {
    renderWithRouterAndClient(
      <RuleDefinition
        definition="reason LIKE blah"
        onEditClicked={() => {
          // Do nothing.
        }}
      />,
    );

    await screen.findByText('reason LIKE blah');

    expect(screen.getByText('reason LIKE blah')).toBeInTheDocument();
  });

  it('given a rule with definition redacted, a message should be displayed', async () => {
    renderWithRouterAndClient(
      <RuleDefinition
        onEditClicked={() => {
          // Do nothing.
        }}
      />,
    );

    await screen.findByText(
      'You do not have permission to view the rule definition.',
    );
    expect(
      screen.getByText(
        'You do not have permission to view the rule definition.',
      ),
    ).toBeInTheDocument();
  });
});
