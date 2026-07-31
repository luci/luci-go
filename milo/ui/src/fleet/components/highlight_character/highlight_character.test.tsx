// Copyright 2026 The LUCI Authors.
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

import { HighlightCharacter } from './highlight_character';

describe('HighlightCharacter', () => {
  it('renders standard text without truncation options', () => {
    render(<HighlightCharacter>Simple Text</HighlightCharacter>);
    expect(screen.getByText('Simple Text')).toBeInTheDocument();
  });

  it('renders highlighted characters correctly', () => {
    const { container } = render(
      <HighlightCharacter highlightIndexes={[0, 1]}>Simple</HighlightCharacter>,
    );
    const spans = container.querySelectorAll('span');
    expect(spans).toHaveLength(6);
    expect(spans[0].textContent).toBe('S');
    expect(spans[1].textContent).toBe('i');
  });

  it('renders middle truncation with default suffix length of 3 for long text', () => {
    const longText = 'chromeos15-row10-rack10-host31';
    const { container } = render(
      <HighlightCharacter truncate="middle">{longText}</HighlightCharacter>,
    );

    const prefix = longText.slice(0, longText.length - 3); // 'chromeos15-row10-rack10-hos'
    const suffix = longText.slice(longText.length - 3); // 't31'

    expect(screen.getByText(prefix)).toBeInTheDocument();
    expect(screen.getByText(suffix)).toBeInTheDocument();

    const flexTypography = container.firstElementChild;
    expect(flexTypography).toHaveStyle({ display: 'flex' });
  });

  it('does not split short text when truncate="middle"', () => {
    const shortText = 'host31';
    render(
      <HighlightCharacter truncate="middle">{shortText}</HighlightCharacter>,
    );

    expect(screen.getByText('host31')).toBeInTheDocument();
  });

  it('supports custom suffixLength in middle truncation', () => {
    const longText = 'chromeos15-row10-rack10-host31';
    render(
      <HighlightCharacter truncate="middle" suffixLength={5}>
        {longText}
      </HighlightCharacter>,
    );

    const prefix = longText.slice(0, longText.length - 5); // 'chromeos15-row10-rack10-h'
    const suffix = longText.slice(longText.length - 5); // 'ost31'

    expect(screen.getByText(prefix)).toBeInTheDocument();
    expect(screen.getByText(suffix)).toBeInTheDocument();
  });

  it('correctly maps highlight indexes across prefix and suffix in middle truncation mode', () => {
    const text = 'chromeos15-row10-rack10-host31';
    // Highlight 'c' (index 0) in prefix and '3' (index 28) in suffix ('t31')
    const { container } = render(
      <HighlightCharacter truncate="middle" highlightIndexes={[0, 28]}>
        {text}
      </HighlightCharacter>,
    );

    // Get all span elements generated for letters in prefix and suffix
    const letterSpans = container.querySelectorAll('span span');
    expect(letterSpans.length).toBeGreaterThan(0);

    // Letter 'c' is the 1st letter in prefix (index 0), letter '3' is the 2nd letter in suffix 't31' (index 28)
    const cSpan = Array.from(letterSpans).find((s) => s.textContent === 'c');
    const threeSpan = Array.from(letterSpans).find(
      (s) => s.textContent === '3',
    );
    const hSpan = Array.from(letterSpans).find((s) => s.textContent === 'h');

    expect(cSpan).toBeDefined();
    expect(threeSpan).toBeDefined();
    expect(hSpan).toBeDefined();

    // cSpan and threeSpan have Emotion generated class for blue color while hSpan does not
    expect(cSpan?.getAttribute('class')).not.toBe(hSpan?.getAttribute('class'));
    expect(threeSpan?.getAttribute('class')).toBe(cSpan?.getAttribute('class'));
  });
});
