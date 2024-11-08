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

import { render, screen } from '@testing-library/react';

import {
  createMockSelectedVariantGroups,
  newMockFailure,
  newMockGroup,
} from '@/clusters/testing_tools/mocks/failures_mock';

import FailuresTableRows from './failures_table_rows';

describe('Test FailureTableRows component', () => {
  it('given a group without children', async () => {
    const mockGroup = newMockGroup({ type: 'leaf', value: 'testgroup' })
      .withFailures(2)
      .withPresubmitRejects(3)
      .withInvocationFailures(4)
      .withCriticalFailuresExonerated(5)
      .build();
    render(
      <table>
        <tbody>
          <FailuresTableRows
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByText(mockGroup.key.value);

    expect(screen.getByText(mockGroup.presubmitRejects)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.invocationFailures)).toBeInTheDocument();
    expect(
      screen.getByText(mockGroup.criticalFailuresExonerated),
    ).toBeInTheDocument();
    expect(screen.getByText(mockGroup.failures)).toBeInTheDocument();
  });

  it('given a group with a presubmit failure then should display failure details', async () => {
    const failure = newMockFailure().ingestedInvocationBlocked().build();
    const mockGroup = newMockGroup({ type: 'leaf', value: 'testgroup' })
      .withFailure(failure)
      .withFailures(2)
      .withPresubmitRejects(3)
      .withInvocationFailures(4)
      .withCriticalFailuresExonerated(5)
      .build();
    render(
      <table>
        <tbody>
          <FailuresTableRows
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByLabelText('Failure invocation id');

    expect(screen.getByText('buildnumber')).toBeInTheDocument();
    expect(screen.getByText('Unexpected')).toBeInTheDocument();
    expect(screen.getByTestId('ungrouped_variants')).toBeInTheDocument();
    expect(screen.getByText('123456 #7')).toBeInTheDocument();
    expect(screen.getByText('Submitted')).toBeInTheDocument();

    expect(screen.getByText(mockGroup.presubmitRejects)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.invocationFailures)).toBeInTheDocument();
    expect(
      screen.getByText(mockGroup.criticalFailuresExonerated),
    ).toBeInTheDocument();
    expect(screen.getByText(mockGroup.failures)).toBeInTheDocument();
  });

  it('given a group with a postsubmit failure then should display failure details', async () => {
    const failure = newMockFailure()
      .withoutPresubmit()
      .exonerateNotCritical()
      .build();
    const mockGroup = newMockGroup({ type: 'leaf', value: 'testgroup' })
      .withFailure(failure)
      .withFailures(2)
      .withInvocationFailures(4)
      .withCriticalFailuresExonerated(5)
      .build();
    render(
      <table>
        <tbody>
          <FailuresTableRows
            project="testproject"
            group={mockGroup}
            selectedVariantGroups={createMockSelectedVariantGroups()}
          />
        </tbody>
      </table>,
    );

    await screen.findByLabelText('Failure invocation id');

    expect(screen.getByText('buildnumber')).toBeInTheDocument();
    expect(screen.getByText('Exonerated')).toBeInTheDocument();
    expect(screen.getByTestId('ungrouped_variants')).toBeInTheDocument();

    expect(screen.getByText(mockGroup.invocationFailures)).toBeInTheDocument();
    expect(
      screen.getByText(mockGroup.criticalFailuresExonerated),
    ).toBeInTheDocument();
    expect(screen.getByText(mockGroup.failures)).toBeInTheDocument();
  });
});
