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

import {
  render,
  screen,
} from '@testing-library/react';

import { PriorityExplanationSection } from './priority_explanation_section';
import { PriorityRecommendation } from '../priority_recommendation';

describe('Test PriorityExplanationSection component', () => {
  it('renders the justification for a bug priority recommendation', async () => {
    const recommendation: PriorityRecommendation = {
      recommendation: {
        priority: 'P1',
        satisfied: true,
        criteria: [
          {
            metricName: 'User Cls Failed Presubmit',
            durationKey: '1d',
            thresholdValue: '10',
            actualValue: '13',
            satisfied: true,
          },
        ],
      },
      justification: [
        {
          priority: 'P0',
          satisfied: false,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              durationKey: '1d',
              thresholdValue: '20',
              actualValue: '13',
              satisfied: false,
            },
          ],
        },
        {
          priority: 'P1',
          satisfied: true,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              durationKey: '1d',
              thresholdValue: '10',
              actualValue: '13',
              satisfied: true,
            },
          ],
        },
        {
          priority: 'P2',
          satisfied: true,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '167',
              satisfied: true,
            },
            {
              metricName: 'Presubmit-blocking Failures Exonerated',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '0',
              satisfied: false,
            },
          ],
        },
      ],
    };

    render(
      <PriorityExplanationSection recommendation={recommendation} />
    );

    await screen.findAllByTestId('priority-explanation-section');

    // Check the go link for LUCI Analysis bug management has been displayed.
    const bugManagementLabel = screen.getByText('go/luci-analysis-bug-management');
    expect(bugManagementLabel).toBeInTheDocument();
    expect(bugManagementLabel.getAttribute('href')).not.toBe('');

    // Check the priority criteria table has been displayed.
    expect(screen.getByTestId('priority-explanation-table')).toBeInTheDocument();
    expect(screen.getAllByTestId('priority-explanation-table-row').length).toBe(3);
    expect(screen.getByText('User Cls Failed Presubmit (1d) (value: 13) \u2265 10')).toBeInTheDocument();

    // Check the "Satisfied?" column.
    const verdicts = screen.getAllByTestId('priority-explanation-table-row-verdict');
    expect(verdicts.length).toBe(3);
    const veridictValues = verdicts.map(verdictElement => verdictElement.textContent);
    expect(veridictValues).toEqual(['No', 'Yes', 'Yes']);

    expect(screen.queryByText(
      "no priority's criteria was satisfied",
      { exact: false },
    )).not.toBeInTheDocument();

    // Check there's a link to update the project config.
    const projectConfigLabel = screen.getByText('project configuration');
    expect(projectConfigLabel).toBeInTheDocument();
    expect(projectConfigLabel.getAttribute('href')).not.toBe('');
  });

  it('shows the justification for no recommended bug priority', async () => {
    const recommendation: PriorityRecommendation = {
      justification: [
        {
          priority: 'P0',
          satisfied: false,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              durationKey: '1d',
              thresholdValue: '20',
              actualValue: '0',
              satisfied: false,
            },
          ],
        },
        {
          priority: 'P1',
          satisfied: false,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              durationKey: '1d',
              thresholdValue: '10',
              actualValue: '0',
              satisfied: false,
            },
          ],
        },
        {
          priority: 'P2',
          satisfied: false,
          criteria: [
            {
              metricName: 'User Cls Failed Presubmit',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '0',
              satisfied: false,
            },
            {
              metricName: 'Presubmit-blocking Failures Exonerated',
              durationKey: '7d',
              thresholdValue: '1',
              actualValue: '0',
              satisfied: false,
            },
          ],
        },
      ],
    };

    render(
      <PriorityExplanationSection recommendation={recommendation} />
    );

    await screen.findAllByTestId('priority-explanation-section');

    // Check the go link for LUCI Analysis bug management has been displayed.
    const bugManagementLabel = screen.getByText('go/luci-analysis-bug-management');
    expect(bugManagementLabel).toBeInTheDocument();
    expect(bugManagementLabel.getAttribute('href')).not.toBe('');

    // Check the priority criteria table has been displayed.
    expect(screen.getByTestId('priority-explanation-table')).toBeInTheDocument();
    expect(screen.getAllByTestId('priority-explanation-table-row').length).toBe(3);

    // Check the "Satisfied?" column.
    const verdicts = screen.getAllByTestId('priority-explanation-table-row-verdict');
    expect(verdicts.length).toBe(3);
    const veridictValues = verdicts.map(verdictElement => verdictElement.textContent);
    expect(veridictValues).toEqual(['No', 'No', 'No']);

    expect(screen.getByText(
      "no priority's criteria was satisfied",
      { exact: false },
    )).toBeInTheDocument();

    // Check there's a link to update the project config.
    const projectConfigLabel = screen.getByText('project configuration');
    expect(projectConfigLabel).toBeInTheDocument();
    expect(projectConfigLabel.getAttribute('href')).not.toBe('');
  });

  it('shows an alert if the project config does not have any priority criteria', async () => {
    const recommendation: PriorityRecommendation = {
      justification: [],
    };

    render(
      <PriorityExplanationSection recommendation={recommendation} />
    );

    await screen.findAllByTestId('priority-explanation-section');

    // Check the go link for LUCI Analysis bug management has been displayed.
    const bugManagementLabel = screen.getByText('go/luci-analysis-bug-management');
    expect(bugManagementLabel).toBeInTheDocument();
    expect(bugManagementLabel.getAttribute('href')).not.toBe('');

    // Check the priority criteria table has NOT been displayed.
    expect(screen.queryByTestId('priority-explanation-table')).not.toBeInTheDocument();

    expect(screen.queryByText(
      "no priority's criteria was satisfied",
      { exact: false },
    )).not.toBeInTheDocument();

    expect(screen.getByText(
      'project configuration does not have priority thresholds specified',
      { exact: false },
    )).toBeInTheDocument();

    // Check there's a link to update the project config.
    const projectConfigLabel = screen.getByText('project configuration');
    expect(projectConfigLabel).toBeInTheDocument();
    expect(projectConfigLabel.getAttribute('href')).not.toBe('');
  });
});
