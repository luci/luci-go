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

import { styled } from '@mui/material';
import { escapeRegExp } from 'lodash-es';

const Container = styled('span')`
  & > * {
    white-space: pre;
  }
`;

export interface HighlightedTextProps {
  /**
   * The text to be rendered.
   */
  readonly text: string;
  /**
   * The substring to be highlighted. Case insensitive.
   */
  readonly highlight: string;
}

/**
 * Renders `text` and highlight the first occurrence of `highlight`.
 */
export function HighlightedText({ text, highlight }: HighlightedTextProps) {
  if (highlight.trim() === '') {
    return (
      <Container>
        <span>{text}</span>
      </Container>
    );
  }

  const re = new RegExp(
    `^((?:.|\n)*?)(${escapeRegExp(highlight)})((?:.|\n)*)$`,
    'i',
  );
  const match = re.exec(text);
  if (!match) {
    return (
      <Container>
        <span>{text}</span>
      </Container>
    );
  }

  const [, prefix, matched, suffix] = match;

  return (
    <Container>
      <span>{prefix}</span>
      <b>{matched}</b>
      <span>{suffix}</span>
    </Container>
  );
}
