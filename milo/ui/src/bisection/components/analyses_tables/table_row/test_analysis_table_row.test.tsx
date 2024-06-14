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

import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import { render, screen } from '@testing-library/react';

import { ANALYSIS_STATUS_DISPLAY_MAP } from '@/bisection/constants';
import { createMockTestAnalysis } from '@/bisection/testing_tools/mocks/test_analysis_mock';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { SuspectVerificationStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { CulpritActionType } from '@/proto/go.chromium.org/luci/bisection/proto/v1/culprits.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestAnalysisTableRow } from './test_analysis_table_row';

describe('<TestAnalysisTableRow />', () => {
  test('if analysis information is displayed', async () => {
    const mockAnalysis: TestAnalysis = createMockTestAnalysis('123');

    render(
      <FakeContextProvider>
        <Table>
          <TableBody>
            <TestAnalysisTableRow analysis={mockAnalysis} />
          </TableBody>
        </Table>
      </FakeContextProvider>,
    );

    await screen.findByTestId('analysis_table_row');

    // Check there is a link to the analysis details page
    const analysisLink = screen.getByTestId('analysis_table_row_analysis_link');
    expect(analysisLink).toBeInTheDocument();
    expect(analysisLink.textContent).toBe(mockAnalysis.analysisId);
    expect(analysisLink.getAttribute('href')).toBe(
      `/ui/bisection/test-analysis/b/${mockAnalysis.analysisId}`,
    );

    // Check the start time is displayed. Example formatted timestamps:
    //    * 12:34:56 Dec, 03 2022 PDT
    //    * 12:34:56 Dec, 03 2022 GMT+10
    const startTimeFormat = new RegExp(
      '^\\d{2}:[0-5]\\d:[0-5]\\d [A-Z][a-z]{2}, [A-Z][a-z]{2} [0-3]\\d \\d{4} [A-Z]+',
    );
    expect(screen.queryAllByText(startTimeFormat)).toHaveLength(1);

    // Check the status and failure type are displayed
    expect(
      screen.getByText(ANALYSIS_STATUS_DISPLAY_MAP[mockAnalysis.status]),
    ).toBeInTheDocument();

    // Check the duration is displayed
    expect(screen.queryAllByTestId('duration')).toHaveLength(2);

    // Check there is a link to the builder
    const builderLink = screen.getByTestId('analysis_table_row_builder_link');
    expect(builderLink).toBeInTheDocument();
    expect(builderLink.textContent).toContain(mockAnalysis.builder!.project);
    expect(builderLink.textContent).toContain(mockAnalysis.builder!.bucket);
    expect(builderLink.textContent).toContain(mockAnalysis.builder!.builder);
    expect(builderLink.getAttribute('href')).not.toBe('');

    // Check there is no link for culprits
    expect(
      screen.queryAllByTestId('analysis_table_row_culprit_link'),
    ).toHaveLength(0);
  });

  test('if culprit information is displayed', async () => {
    const mockAnalysis = TestAnalysis.fromPartial({
      ...createMockTestAnalysis('125'),
      culprit: {
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'abc123abc123',
          position: 307,
        },
        reviewTitle: 'Added new feature to improve testing',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/123456',
        verificationDetails: {
          status: SuspectVerificationStatus.CONFIRMED_CULPRIT,
        },
        culpritAction: [
          {
            actionType: CulpritActionType.CULPRIT_AUTO_REVERTED,
            revertClUrl:
              'https://chromium-review.googlesource.com/placeholder/+/123457',
          },
        ],
      },
    });

    render(
      <FakeContextProvider>
        <Table>
          <TableBody>
            <TestAnalysisTableRow analysis={mockAnalysis} />
          </TableBody>
        </Table>
      </FakeContextProvider>,
    );

    await screen.findByTestId('analysis_table_row');

    // Check there is a link for each culprit
    expect(
      screen.queryAllByTestId('analysis_table_row_culprit_link'),
    ).toHaveLength(1);

    // Check there is an icon for the auto-revert action.
    expect(
      screen.getByTestId('culprit-action-icon-auto-reverted'),
    ).toBeInTheDocument();
  });
});
