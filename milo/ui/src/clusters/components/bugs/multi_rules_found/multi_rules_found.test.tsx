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

import { screen } from '@testing-library/react';

import { renderWithRouter } from '@/clusters/testing_tools/libs/mock_router';

import MultiRulesFound from './multi_rules_found';

describe('test MultiRulesFound component', () => {
  it('given a list rules, should display links to their details', async () => {
    const rules: string[] = [
      'projects/chrome/rules/123456',
      'projects/chrome/rules/789010',
    ];
    renderWithRouter(
      <MultiRulesFound bugSystem="monorail" bugId="123456" rules={rules} />,
    );

    await screen.findByText('Multiple projects ', { exact: false });

    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getAllByRole('link').length).toEqual(2);
  });
});
