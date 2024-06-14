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

import {
  ANALYSIS_STATUS_DISPLAY_MAP,
  BUILD_FAILURE_TYPE_DISPLAY_MAP,
} from '@/bisection/constants';
import { createMockAnalysis } from '@/bisection/testing_tools/mocks/analysis_mock';
import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { CulpritActionType } from '@/proto/go.chromium.org/luci/bisection/proto/v1/culprits.pb';

import { AnalysisOverview } from './analysis_overview';

describe('<AnalysisOverview />', () => {
  test('if all analysis summary details are displayed', async () => {
    const mockAnalysis = createMockAnalysis('1');

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    const expectedStaticFields = [
      ['analysis ID', 'analysisId'],
      ['buildbucket ID', 'firstFailedBbid'],
      ['failure type', 'buildFailureType'],
    ];

    // check static field labels and values are displayed
    expectedStaticFields.forEach(([label, property]) => {
      const fieldLabel = screen.getByText(new RegExp(`^(${label})$`, 'i'));
      expect(fieldLabel).toBeInTheDocument();
      let valueContent = `${mockAnalysis[property as keyof Analysis]}`;
      if (property === 'status') {
        valueContent = ANALYSIS_STATUS_DISPLAY_MAP[mockAnalysis['status']];
      }
      if (property === 'buildFailureType') {
        valueContent =
          BUILD_FAILURE_TYPE_DISPLAY_MAP[mockAnalysis['buildFailureType']];
      }
      expect(fieldLabel.nextSibling?.textContent).toBe(valueContent);
    });

    const statusFieldLabel = screen.getByText('Status');
    expect(statusFieldLabel.nextSibling?.textContent).toBe('Culprit found');

    // check the timestamp cells are displayed
    const timestampFields = ['created time', 'end time'];
    // Example formatted timestamps:
    //    * 12:34:56 Dec, 03 2022 PDT
    //    * 12:34:56 Dec, 03 2022 GMT+10
    const timeFormat = new RegExp(
      '^\\d{2}:[0-5]\\d:[0-5]\\d [A-Z][a-z]{2}, [A-Z][a-z]{2} [0-3]\\d \\d{4} [A-Z]+',
    );
    timestampFields.forEach((field) => {
      const fieldLabel = screen.getByText(new RegExp(`^(${field})$`, 'i'));
      expect(fieldLabel).toBeInTheDocument();
      // just check the format; the actual value will depend on timezone
      expect(fieldLabel.nextSibling?.textContent).toMatch(timeFormat);
    });

    // check the builder is displayed correctly
    expect(mockAnalysis.builder).not.toBe(undefined);
    const fieldLabel = screen.getByText(new RegExp('^(builder)$', 'i'));
    expect(fieldLabel).toBeInTheDocument();
    const builderText = fieldLabel.nextSibling?.textContent || '';
    expect(builderText).toContain(mockAnalysis.builder!.project);
    expect(builderText).toContain(mockAnalysis.builder!.bucket);
    expect(builderText).toContain(mockAnalysis.builder!.builder);

    // check the suspect range is displayed correctly
    verifySuspectRangeLinks(mockAnalysis);

    // check there are no bug links; there are no culprits nor culprit actions
    expect(screen.queryByText('Related bugs')).not.toBeInTheDocument();
    expect(screen.queryAllByTestId('analysis_overview_bug_link')).toHaveLength(
      0,
    );
  });

  test('if there is a culprit for the analysis, then it should be the suspect range', async () => {
    const mockAnalysis = Analysis.fromPartial({
      ...createMockAnalysis('3'),
      culprits: [
        {
          commit: {
            host: 'testHost',
            project: 'testProject',
            ref: 'test/ref/dev',
            id: 'ghi789ghi789',
            position: 523,
          },
          reviewTitle: 'Added new feature to improve testing',
          reviewUrl:
            'https://chromium-review.googlesource.com/placeholder/+/123456',
          culpritAction: [
            {
              actionType: CulpritActionType.BUG_COMMENTED,
              bugUrl: 'https://crbug.com/testProject/11223344',
            },
            {
              actionType: CulpritActionType.BUG_COMMENTED,
              bugUrl: 'https://buganizer.corp.google.com/99887766',
            },
          ],
          verificationDetails: {
            status: 'Confirmed Culprit',
          },
        },
      ],
    });

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check related bug links are displayed
    expect(
      screen.getByText(new RegExp('^(related bugs)$', 'i')),
    ).toBeInTheDocument();
    mockAnalysis.culprits.forEach((culprit) => {
      culprit.culpritAction?.forEach((action) => {
        expect(screen.getByText(action.bugUrl!).getAttribute('href')).toBe(
          action.bugUrl,
        );
      });
    });

    // check the suspect range is displayed correctly
    verifySuspectRangeLinks(mockAnalysis);
  });

  // eslint-disable-next-line jest/expect-expect
  test('if there is a culprit for only the nth section analysis, then it should be the suspect range', async () => {
    let mockAnalysis = createMockAnalysis('4');
    mockAnalysis = Analysis.fromPartial({
      ...mockAnalysis,
      nthSectionResult: {
        ...mockAnalysis.nthSectionResult,
        suspect: {
          commit: {
            host: 'testHost',
            project: 'testProject',
            ref: 'test/ref/dev',
            id: 'jkl012jkl012',
            position: 624,
          },
          reviewUrl: 'http://this/is/review/url',
          reviewTitle: 'Review title',
          verificationDetails: {
            status: 'Under Verification',
          },
        },
      },
    });

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLinks(mockAnalysis);
  });

  // eslint-disable-next-line jest/expect-expect
  test('if there is no data for the suspect range, then the table cell should be empty', async () => {
    const mockAnalysis = Analysis.fromPartial({
      ...createMockAnalysis('5'),
      nthSectionResult: undefined,
    });

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLinks(mockAnalysis);
  });
});

function verifySuspectRangeLinks(analysis: Analysis) {
  // check the label for the suspect range has been rendered
  const suspectRangeLabel = screen.getByText(
    new RegExp('^(suspect range)$', 'i'),
  );
  expect(suspectRangeLabel).toBeInTheDocument();

  // check the suspect range link element(s) have been rendered
  const suspectRangeLinks = screen.queryAllByTestId(
    'analysis_overview_suspect_range',
  );

  if (analysis.culprits && analysis.culprits.length > 0) {
    expect(suspectRangeLinks).toHaveLength(analysis.culprits.length);
    suspectRangeLinks.forEach((suspectRangeLink) => {
      expect(suspectRangeLink.textContent).not.toBe('');
      expect(suspectRangeLink.getAttribute('href')).not.toBe('');
    });
    return;
  } else if (analysis.nthSectionResult) {
    if (analysis.nthSectionResult.suspect) {
      expect(suspectRangeLinks).toHaveLength(1);
      const nthCulpritLink = suspectRangeLinks[0];
      expect(analysis.nthSectionResult.suspect.commit!.id).toContain(
        nthCulpritLink.textContent,
      );
      expect(nthCulpritLink.getAttribute('href')).not.toBe('');
      return;
    } else if (analysis.nthSectionResult.remainingNthSectionRange) {
      expect(suspectRangeLinks).toHaveLength(1);
      const nthSectionRangeLink = suspectRangeLinks[0];
      expect(nthSectionRangeLink.textContent).toMatch(
        new RegExp('^(.+) ... (.+)$'),
      );
      expect(nthSectionRangeLink.getAttribute('href')).not.toBe('');
      return;
    }
  }

  expect(suspectRangeLinks).toHaveLength(0);
}
