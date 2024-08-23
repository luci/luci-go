// Copyright 2024 The LUCI Authors.
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

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { LogSnippetRow } from './log_snippet_row';

describe('<LogSnippetRow />', () => {
  it('can display snippet and matches', () => {
    render(
      <LogSnippetRow
        artifact={{
          name: 'artifactName',
          testStatus: TestStatus.PASS,
          partitionTime: '2024-08-20T14:30:00Z',
          snippet: 'abc',
          matches: [
            { startIndex: 0, endIndex: 1 },
            { startIndex: 1, endIndex: 3 },
          ],
        }}
      />,
    );
    const t1 = screen.getByText('a');
    expect(t1).toHaveStyle({ backgroundColor: '#ceead6', fontWeight: '700' });
    const t2 = screen.getByText('bc');
    expect(t2).toHaveStyle({ backgroundColor: '#ceead6', fontWeight: '700' });
  });

  it('can display snippet and matches with non-match part', () => {
    render(
      <LogSnippetRow
        artifact={{
          name: 'artifactName',
          testStatus: TestStatus.PASS,
          partitionTime: '2024-08-20T14:30:00Z',
          snippet: 'abcdef',
          matches: [{ startIndex: 1, endIndex: 3 }],
        }}
      />,
    );
    const t1 = screen.getByText('a');
    expect(t1).toBeInTheDocument();
    const b2 = screen.getByText('bc');
    expect(b2).toHaveStyle({ backgroundColor: '#ceead6', fontWeight: '700' });
    const t3 = screen.getByText('def');
    expect(t3).toBeInTheDocument();
  });

  it('can display snippet and matches with multi-bytes string', () => {
    render(
      <LogSnippetRow
        artifact={{
          name: 'artifactName',
          testStatus: TestStatus.PASS,
          partitionTime: '2024-08-20T14:30:00Z',
          snippet: '😊🌞',
          matches: [{ startIndex: 4, endIndex: 8 }],
        }}
      />,
    );

    const textElement = screen.getByText('😊');
    expect(textElement).toBeInTheDocument();
    const b1 = screen.getByText('🌞');
    expect(b1).toHaveStyle({ backgroundColor: '#ceead6', fontWeight: '700' });
  });
});
