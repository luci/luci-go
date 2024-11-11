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

import { fireEvent, render, screen } from '@testing-library/react';

import {
  createDefaultMockFailureGroup,
  createDefaultMockFailureGroupWithChildren,
  createMockSelectedVariantGroups,
} from '@/clusters/testing_tools/mocks/failures_mock';

import FailuresTableGroup from './failures_table_group';

describe('Test FailureTableGroup component', () => {
  it('given a group without children then should display 1 row', async () => {
    const mockGroup = createDefaultMockFailureGroup();
    render(
      <table>
        <tbody>
          <FailuresTableGroup
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByText(mockGroup.key.value);

    expect(screen.getByText(mockGroup.key.value)).toBeInTheDocument();
  });

  it('given a group with children then should display just the group when not expanded', async () => {
    const mockGroup = createDefaultMockFailureGroupWithChildren();
    render(
      <table>
        <tbody>
          <FailuresTableGroup
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByText(mockGroup.key.value);

    expect(screen.getAllByRole('row')).toHaveLength(1);
  });

  it('given a test name group it should show a test history link', async () => {
    const mockGroup = createDefaultMockFailureGroupWithChildren();
    mockGroup.key = {
      type: 'test',
      value: 'ninja://package/sometest.Blah?a=1',
    };
    mockGroup.commonVariant = {
      def: {
        k1: 'v1',
        // Consider a variant with special characters.
        'key %+': 'value %+',
      },
    };
    render(
      <table>
        <tbody>
          <FailuresTableGroup
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByText(mockGroup.key.value);

    expect(screen.getByLabelText('Test history link')).toBeInTheDocument();
    expect(screen.getByLabelText('Test history link')).toHaveAttribute(
      'href',
      '/ui/test/testproject/ninja%3A%2F%2Fpackage%2Fsometest.Blah%3Fa%3D1?q=V%3Ak1%3Dv1%20V%3Akey%2520%2525%252B%3Dvalue%2520%2525%252B',
    );
    expect(screen.getAllByRole('row')).toHaveLength(1);
  });

  it('given a group with children then should display all when expanded', async () => {
    const mockGroup = createDefaultMockFailureGroupWithChildren();
    render(
      <table>
        <tbody>
          <FailuresTableGroup
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByText(mockGroup.key.value);

    fireEvent.click(screen.getByLabelText('Expand group'));

    await screen.findByText(mockGroup.children[2].failures);

    expect(screen.getAllByRole('row')).toHaveLength(4);
  });
});
