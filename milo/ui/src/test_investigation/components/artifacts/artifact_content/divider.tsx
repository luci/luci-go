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

import { Box, Button } from '@mui/material';
import { CSSProperties } from 'react';

interface DividerProps {
  numLines: number;
  onExpandStart: ((numLines: number) => void) | undefined;
  onExpandEnd: ((numLines: number) => void) | undefined;
}

export function Divider({
  numLines,
  onExpandEnd,
  onExpandStart,
}: DividerProps) {
  const buttonSx: CSSProperties = {
    backgroundColor: '#fff',
    border: 'solid 1px #aaa',
    margin: 0,
    padding: '2px 12px',
    minWidth: '20px',
    textTransform: 'none',
  };
  const noTopBorderSx: CSSProperties = {
    ...buttonSx,
    borderTop: 'none',
    borderTopLeftRadius: 0,
    borderTopRightRadius: 0,
  };
  const noBottomBorderSx: CSSProperties = {
    ...buttonSx,
    borderBottom: 'none',
    borderBottomLeftRadius: 0,
    borderBottomRightRadius: 0,
  };

  if (numLines <= 0) {
    return null;
  }
  // Because the buttons are  position: relative, they are outside the flow of
  // the document and the margins on the container are the only thing that
  // reserves the space for them.  Thus we need to handle all of the possible
  // configurations of buttons that can be present in the margin calculation.
  const dividerMargin = `${
    onExpandStart ? (!onExpandEnd || numLines > 10 ? '36px' : '18px') : 0
  } 0 ${onExpandEnd ? (!onExpandStart || numLines > 10 ? '36px' : '18px') : 0}`;

  return (
    <Box
      sx={{
        height: onExpandEnd && onExpandStart ? '4px' : 0,
        backgroundColor: 'divider',
        m: dividerMargin,
        borderTop: onExpandStart && onExpandEnd ? 'solid 1px #aaa' : 'none',
        borderBottom: onExpandStart && onExpandEnd ? 'solid 1px #aaa' : 'none',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 2,
          position: 'relative',
          top: onExpandStart ? (numLines > 10 ? '-29px' : '-14px') : '1px',
        }}
      >
        <Button
          onClick={() =>
            onExpandStart
              ? onExpandStart(numLines)
              : onExpandEnd && onExpandEnd(numLines)
          }
          size="small"
          sx={
            !onExpandEnd
              ? noBottomBorderSx
              : !onExpandStart
                ? noTopBorderSx
                : buttonSx
          }
        >
          +{numLines} hidden lines
        </Button>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
          {onExpandStart && numLines > 10 && (
            <Button
              onClick={() => onExpandStart(10)}
              sx={noBottomBorderSx}
              size="small"
            >
              +10
            </Button>
          )}
          {onExpandEnd && numLines > 10 && (
            <Button
              onClick={() => onExpandEnd && onExpandEnd(10)}
              sx={noTopBorderSx}
              size="small"
            >
              +10
            </Button>
          )}
        </Box>
      </Box>
    </Box>
  );
}
