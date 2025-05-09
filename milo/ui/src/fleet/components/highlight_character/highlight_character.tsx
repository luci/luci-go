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

export const HighlightCharacter = ({
  children,
  highlightIndexes,
  ...typographyProps
}: TypographyProps & {
  children: string;
  highlightIndexes?: number[];
}) => (
  <Typography {...typographyProps}>
    {highlightIndexes && highlightIndexes.length > 0
      ? children.split('').map((letter, i) => (
          <span
            key={`item-${children}-letter-${i}`}
            css={{
              color: highlightIndexes.includes(i) ? colors.blue[600] : '',
            }}
          >
            {letter}
          </span>
        ))
      : children}
  </Typography>
);
