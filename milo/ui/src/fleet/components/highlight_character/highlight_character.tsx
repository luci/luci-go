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

import { Typography, TypographyProps } from '@mui/material';

import { colors } from '@/fleet/theme/colors';

export interface HighlightCharacterProps extends TypographyProps {
  children: string;
  highlightIndexes?: number[];
  truncate?: 'none' | 'end' | 'middle';
  suffixLength?: number;
}

export const HighlightCharacter = ({
  children,
  highlightIndexes,
  truncate = 'end',
  suffixLength = 3,
  ...typographyProps
}: HighlightCharacterProps) => {
  const renderLetters = (
    text: string,
    highlights: number[] = [],
    startIndex: number = 0,
  ) => {
    if (!highlights || highlights.length === 0) {
      return text;
    }
    return text.split('').map((letter, i) => (
      <span
        key={`item-${children}-letter-${startIndex + i}`}
        css={{
          color: highlights.includes(i) ? colors.blue[600] : '',
        }}
      >
        {letter}
      </span>
    ));
  };

  // Only perform middle truncation if string is longer than suffix + ellipsis ("..." is 3 chars)
  if (truncate === 'middle' && children.length > suffixLength + 3) {
    const splitIndex = children.length - suffixLength;
    const prefix = children.slice(0, splitIndex);
    const suffix = children.slice(splitIndex);

    const prefixHighlights = highlightIndexes
      ? highlightIndexes.filter((i) => i < splitIndex)
      : [];
    const suffixHighlights = highlightIndexes
      ? highlightIndexes
          .filter((i) => i >= splitIndex)
          .map((i) => i - splitIndex)
      : [];

    return (
      <Typography
        {...typographyProps}
        sx={{
          display: 'flex',
          minWidth: 0,
          width: '100%',
          ...typographyProps.sx,
        }}
      >
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            minWidth: 0,
            flexShrink: 1,
          }}
        >
          {renderLetters(prefix, prefixHighlights, 0)}
        </span>
        <span
          style={{
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          {renderLetters(suffix, suffixHighlights, splitIndex)}
        </span>
      </Typography>
    );
  }

  return (
    <Typography
      {...typographyProps}
      sx={{
        display: 'block',
        ...(truncate === 'end' && {
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
        }),
        ...typographyProps.sx,
      }}
    >
      {renderLetters(children, highlightIndexes, 0)}
    </Typography>
  );
};
