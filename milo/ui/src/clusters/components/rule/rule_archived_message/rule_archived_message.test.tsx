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

import '@testing-library/jest-dom';
import 'node-fetch';

import { screen } from '@testing-library/react';

import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';

import RuleArchivedMessage from './rule_archived_message';

describe('Test RuleArchivedMessage component', () => {
  it('should display the rule archived message', async () => {
    renderWithRouterAndClient(<RuleArchivedMessage />);

    await screen.findByText('This Rule is Archived');

    expect(
      screen.getByText('If you came here', { exact: false }),
    ).toBeInTheDocument();
  });
});
