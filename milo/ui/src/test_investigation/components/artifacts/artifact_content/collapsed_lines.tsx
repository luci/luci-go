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

import { Box } from '@mui/material';
import { useState } from 'react';

import { Divider } from './divider';
import { TextBox } from './text_box';

export function CollapsedLines({
  lines,
  isStart,
  isEnd,
}: {
  lines: string[];
  isStart: boolean;
  isEnd: boolean;
}) {
  const [startExpandedCount, setStartExpandedCount] = useState(0);
  const [endExpandedCount, setEndExpandedCount] = useState(0);

  const handleExpandStart = (count: number) => {
    setStartExpandedCount((prev) => Math.min(prev + count, lines.length));
  };
  const handleExpandEnd = (count: number) => {
    setEndExpandedCount((prev) => Math.min(prev + count, lines.length));
  };

  return (
    <Box>
      {startExpandedCount > 0 && (
        <TextBox lines={lines.slice(0, startExpandedCount)} />
      )}
      <Divider
        numLines={lines.length - startExpandedCount - endExpandedCount}
        onExpandStart={isStart ? undefined : handleExpandStart}
        onExpandEnd={isEnd ? undefined : handleExpandEnd}
      />
      {endExpandedCount > 0 && (
        <TextBox lines={lines.slice(-endExpandedCount)} />
      )}
    </Box>
  );
}
