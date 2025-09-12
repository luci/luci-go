// Copyright 2025 The LUCI Authors.
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

import { createMockGenAiSuspect } from '@/bisection/testing_tools/mocks/gen_ai_suspect_mocks';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import {
  GenAiAnalysisResult,
  GenAiSuspect,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/genai.pb';

import { GenAiAnalysisTable } from './gen_ai_analysis_table';

describe('<GenAiAnalysisTable />', () => {
  test('if an appropriate message is displayed for no analysis', async () => {
    render(<GenAiAnalysisTable />);

    await screen.findByTestId('genai-analysis-table');

    expect(screen.queryAllByRole('link')).toHaveLength(0);
    expect(
      screen.getByText('No AI analysis available for this failure.'),
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed when analysis is in progress', async () => {
    render(
      <GenAiAnalysisTable
        result={{ status: AnalysisStatus.RUNNING } as GenAiAnalysisResult}
      />,
    );
    await screen.findByTestId('genai-analysis-table');
    expect(screen.getByText('AI analysis is in progress.')).toBeInTheDocument();
  });

  test('if an appropriate message is displayed when no suspects are found', async () => {
    render(
      <GenAiAnalysisTable
        result={{ status: AnalysisStatus.NOTFOUND } as GenAiAnalysisResult}
      />,
    );
    await screen.findByTestId('genai-analysis-table');
    expect(
      screen.getByText('No suspects found by AI Analysis.'),
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed when no suspects are pressent in the result', async () => {
    render(
      <GenAiAnalysisTable
        result={
          {
            status: AnalysisStatus.FOUND,
            suspect: undefined,
          } as GenAiAnalysisResult
        }
      />,
    );
    await screen.findByTestId('genai-analysis-table');
    expect(
      screen.getByText('No suspects found by AI Analysis.'),
    ).toBeInTheDocument();
  });

  test('if an unverfied suspect is displayed', async () => {
    const mockSuspect: GenAiSuspect = createMockGenAiSuspect(false);
    render(
      <GenAiAnalysisTable
        result={
          {
            status: AnalysisStatus.FOUND,
            suspect: mockSuspect,
          } as GenAiAnalysisResult
        }
      />,
    );
    await screen.findByTestId('genai-analysis-table');
    const suspectReviewLink = screen.getByRole('link');
    expect(suspectReviewLink.getAttribute('href')).toBe(mockSuspect.reviewUrl);
    expect(screen.getByText(mockSuspect.reviewTitle)).toBeInTheDocument();

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('if a verfied suspect is displayed', async () => {
    const mockSuspect = createMockGenAiSuspect(true);
    render(
      <GenAiAnalysisTable
        result={
          {
            status: AnalysisStatus.FOUND,
            suspect: mockSuspect,
          } as GenAiAnalysisResult
        }
      />,
    );
    await screen.findByTestId('genai-analysis-table');
    const suspectReviewLink = screen.getByRole('link');
    expect(suspectReviewLink.getAttribute('href')).toBe(mockSuspect.reviewUrl);
    expect(screen.getByText(mockSuspect.reviewTitle)).toBeInTheDocument();

    expect(screen.getAllByRole('button')).toHaveLength(1);
  });
});
