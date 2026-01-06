// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may not use this file except in compliance with the License.
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

import { TestGenAiAnalysisResult } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

import { GenAiTestAnalysisTable } from './gen_ai_test_analysis_table';

function createMockGenAiTestSuspect(score: number) {
  return {
    reviewUrl: `https://chromium-review.googlesource.com/placeholder/${score}`,
    reviewTitle: `[Test] Suspect with score ${score}`,
    justification: `this is a justification for score ${score}`,
    confidenceScore: score,
  };
}

describe('<GenAiTestAnalysisTable />', () => {
  test('if an appropriate message is displayed for no analysis', async () => {
    render(<GenAiTestAnalysisTable />);

    await screen.findByTestId('genai-test-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('No AI analysis available for this failure.'),
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed when analysis is in progress', async () => {
    render(
      <GenAiTestAnalysisTable
        result={TestGenAiAnalysisResult.fromPartial({
          status: AnalysisStatus.RUNNING,
        })}
      />,
    );
    await screen.findByTestId('genai-test-analysis-table');
    expect(screen.getByText('AI analysis is in progress.')).toBeInTheDocument();
  });

  test('if an appropriate message is displayed when no suspects are found', async () => {
    render(
      <GenAiTestAnalysisTable
        result={TestGenAiAnalysisResult.fromPartial({
          status: AnalysisStatus.NOTFOUND,
        })}
      />,
    );
    await screen.findByTestId('genai-test-analysis-table');
    expect(
      screen.getByText('No suspects found by AI Analysis.'),
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed when no suspects are present in the result', async () => {
    render(
      <GenAiTestAnalysisTable
        result={TestGenAiAnalysisResult.fromPartial({
          status: AnalysisStatus.FOUND,
          suspects: [],
        })}
      />,
    );
    await screen.findByTestId('genai-test-analysis-table');
    expect(
      screen.getByText('No suspects found by AI Analysis.'),
    ).toBeInTheDocument();
  });

  test('if suspects are displayed correctly', async () => {
    const mockSuspect1 = createMockGenAiTestSuspect(80);
    const mockSuspect2 = createMockGenAiTestSuspect(90);
    render(
      <GenAiTestAnalysisTable
        result={TestGenAiAnalysisResult.fromPartial({
          status: AnalysisStatus.FOUND,
          suspects: [mockSuspect1, mockSuspect2],
        })}
      />,
    );
    await screen.findByTestId('genai-test-analysis-table');

    const rows = screen.getAllByRole('row');
    // Header row + two suspect rows
    expect(rows).toHaveLength(3);

    // Check that the suspects are sorted by confidence score (descending)
    expect(rows[1].innerHTML.includes(mockSuspect2.reviewTitle)).toBe(true);
    expect(rows[2].innerHTML.includes(mockSuspect1.reviewTitle)).toBe(true);

    // Check that the confidence scores are displayed as integers
    expect(screen.getByText('90')).toBeInTheDocument();
    expect(screen.getByText('80')).toBeInTheDocument();

    const suspectReviewLinks = screen.getAllByRole('link');
    expect(suspectReviewLinks).toHaveLength(2);
    expect(suspectReviewLinks[0].getAttribute('href')).toBe(
      mockSuspect2.reviewUrl,
    );
    expect(suspectReviewLinks[1].getAttribute('href')).toBe(
      mockSuspect1.reviewUrl,
    );
    expect(screen.getByText(mockSuspect1.reviewTitle)).toBeInTheDocument();
    expect(screen.getByText(mockSuspect2.reviewTitle)).toBeInTheDocument();
    expect(screen.getByText(mockSuspect1.justification)).toBeInTheDocument();
    expect(screen.getByText(mockSuspect2.justification)).toBeInTheDocument();
  });
});
