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

import { Analysis } from '../../services/luci_bisection';
import { AnalysisOverview } from './analysis_overview';
import { createMockAnalysis } from '../../testing_tools/mocks/analysis_mock';

describe('Test AnalysisOverview component', () => {
  test('if all analysis summary details are displayed', async () => {
    const mockAnalysis = createMockAnalysis('1');

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    const expectedStaticFields = [
      ['analysis ID', 'analysisId'],
      ['status', 'status'],
      ['buildbucket ID', 'firstFailedBbid'],
      ['failure type', 'buildFailureType'],
    ];

    // check static field labels and values are displayed
    expectedStaticFields.forEach(([label, property]) => {
      const fieldLabel = screen.getByText(new RegExp(`^(${label})$`, 'i'));
      expect(fieldLabel).toBeInTheDocument();
      expect(fieldLabel.nextSibling?.textContent).toBe(
        `${mockAnalysis[property as keyof Analysis]}`
      );
    });

    // check the builder is displayed correctly
    const fieldLabel = screen.getByText(new RegExp(`^(builder)$`, 'i'));
    expect(fieldLabel).toBeInTheDocument();
    const builderText = fieldLabel.nextSibling?.textContent || '';
    expect(builderText).toContain(mockAnalysis.builder.project);
    expect(builderText).toContain(mockAnalysis.builder.bucket);
    expect(builderText).toContain(mockAnalysis.builder.builder);

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(mockAnalysis);

    // check related bug links are displayed
    expect(
      screen.getByText(new RegExp('^(related bugs)$', 'i'))
    ).toBeInTheDocument();
    mockAnalysis.culpritAction?.forEach((action) => {
      if (action.bugUrl) {
        expect(screen.getByText(action.bugUrl).getAttribute('href')).toBe(
          action.bugUrl
        );
      }
    });
  });

  test('if there are no bugs, then related bugs section is not shown', async () => {
    let mockAnalysis = createMockAnalysis('2');
    mockAnalysis.culpritAction = [];

    const { container } = render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check there are no bug links
    expect(screen.queryByText('Related bugs')).not.toBeInTheDocument();
    expect(container.getElementsByClassName('bugLink')).toHaveLength(0);
  });

  test('if there is a culprit for the analysis, then it should be the suspect range', async () => {
    let mockAnalysis = createMockAnalysis('3');
    mockAnalysis.culprit = {
      host: 'testHost',
      project: 'testProject',
      ref: 'test/ref/dev',
      id: 'ghi789ghi789',
      position: '523',
    };

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(mockAnalysis);
  });

  test('if there is a culprit for only the nth section analysis, then it should be the suspect range', async () => {
    let mockAnalysis = createMockAnalysis('4');
    mockAnalysis.nthSectionResult!.culprit = {
      host: 'testHost',
      project: 'testProject',
      ref: 'test/ref/dev',
      id: 'jkl012jkl012',
      position: '624',
    };

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(mockAnalysis);
  });

  test('if there is no data for the suspect range, then the table cell should be empty', async () => {
    let mockAnalysis = createMockAnalysis('5');
    mockAnalysis.nthSectionResult = undefined;

    render(<AnalysisOverview analysis={mockAnalysis} />);

    await screen.findByTestId('analysis_overview_table_body');

    // check the suspect range is displayed correctly
    verifySuspectRangeLink(mockAnalysis);
  });
});

function verifySuspectRangeLink(analysis: Analysis) {
  // check the label for the suspect range has been rendered
  const suspectRangeLabel = screen.getByText(
    new RegExp('^(suspect range)$', 'i')
  );
  expect(suspectRangeLabel).toBeInTheDocument();

  // check the suspect range link element has been rendered
  const suspectRangeLink = screen.getByTestId(
    'analysis_overview_suspect_range'
  );
  expect(suspectRangeLink).toBeInTheDocument();

  const linkText = suspectRangeLink.textContent;

  let targetShouldBeEmpty = true;
  if (analysis.culprit) {
    expect(analysis.culprit.id).toContain(linkText);
    targetShouldBeEmpty = false;
  } else if (analysis.nthSectionResult) {
    if (analysis.nthSectionResult.culprit) {
      expect(analysis.nthSectionResult.culprit.id).toContain(linkText);
      targetShouldBeEmpty = false;
    } else if (analysis.nthSectionResult.remainingNthSectionRange) {
      expect(linkText).toMatch(new RegExp('^(.+) ... (.+)$'));
      targetShouldBeEmpty = false;
    }
  }

  const linkTarget = suspectRangeLink.getAttribute('href');
  if (targetShouldBeEmpty) {
    expect(linkTarget).toBe('');
  } else {
    expect(linkTarget).not.toBe('');
  }
}
