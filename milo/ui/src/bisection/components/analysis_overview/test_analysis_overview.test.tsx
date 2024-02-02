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

import { createMockTestAnalysis } from '@/bisection/testing_tools/mocks/test_analysis_mock';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { SuspectVerificationStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

import { TestAnalysisOverview } from './test_analysis_overview';

describe('<AnalysisOverview />', () => {
  test('if all analysis summary details are displayed', async () => {
    const mockAnalysis = createMockTestAnalysis('1');

    render(<TestAnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    const expectedStaticFields = [
      ['analysis ID', mockAnalysis.analysisId],
      ['failure type', 'TEST'],
    ];

    // check static field labels and values are displayed
    expectedStaticFields.forEach(([label, property]) => {
      const fieldLabel = screen.getByText(new RegExp(`^(${label})$`, 'i'));
      expect(fieldLabel).toBeInTheDocument();
      expect(fieldLabel.nextSibling?.textContent).toBe(`${property}`);
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
    const fieldLabel = screen.getByText(new RegExp('^(builder)$', 'i'));
    expect(fieldLabel).toBeInTheDocument();
    const builderText = fieldLabel.nextSibling?.textContent || '';
    expect(builderText).toContain(mockAnalysis.builder!.project);
    expect(builderText).toContain(mockAnalysis.builder!.bucket);
    expect(builderText).toContain(mockAnalysis.builder!.builder);

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(
      true,
      'abc123a ... def456d',
      'https://testHost/testProject/+log/abc123abc123..def456def456',
    );
  });

  test('if there is a culprit for the analysis, then it should be the suspect range', async () => {
    const mockAnalysis = TestAnalysis.fromPartial({
      ...createMockTestAnalysis('3'),
      culprit: {
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
        verificationDetails: {
          status: SuspectVerificationStatus.CONFIRMED_CULPRIT,
        },
      },
    });

    render(<TestAnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');
    // check the suspect range is displayed correctly
    verifySuspectRangeLink(
      true,
      'ghi789g',
      'https://testHost/testProject/+/ghi789ghi789',
    );
  });

  // eslint-disable-next-line jest/expect-expect
  test('if there is a culprit for only the nth section analysis, then it should be the suspect range', async () => {
    let mockAnalysis = createMockTestAnalysis('4');
    mockAnalysis = TestAnalysis.fromPartial({
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
            status: SuspectVerificationStatus.UNDER_VERIFICATION,
          },
        },
      },
    });

    render(<TestAnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(
      true,
      'jkl012j',
      'https://testHost/testProject/+/jkl012jkl012',
    );
  });

  // eslint-disable-next-line jest/expect-expect
  test('if there is no data for the suspect range, then the table cell should be empty', async () => {
    const mockAnalysis = TestAnalysis.fromPartial({
      ...createMockTestAnalysis('5'),
      nthSectionResult: undefined,
    });

    render(<TestAnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(false, '', '');
  });
});

function verifySuspectRangeLink(
  hasLink: boolean,
  textContent: string,
  href: string,
) {
  // check the label for the suspect range has been rendered
  const suspectRangeLabel = screen.getByText(
    new RegExp('^(suspect range)$', 'i'),
  );
  expect(suspectRangeLabel).toBeInTheDocument();

  // check the suspect range link element(s) have been rendered
  const suspectRangeLinks = screen.queryAllByTestId(
    'analysis_overview_suspect_range',
  );
  expect(suspectRangeLinks).toHaveLength(hasLink ? 1 : 0);
  if (hasLink) {
    expect(suspectRangeLinks[0].textContent).toBe(textContent);
    expect(suspectRangeLinks[0].getAttribute('href')).toBe(href);
  }
}
