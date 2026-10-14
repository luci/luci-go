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

import { Box } from '@mui/material';
import { ReactNode } from 'react';

import {
  controlHeights,
  fontFamilies,
  fontSizes,
  radii,
  StatusTone,
  statusTones,
} from '@/fleet/theme/tokens';

import { taskResultTone } from './task_result_tone';

export interface StatusPillProps {
  tone: StatusTone;
  children: ReactNode;
  /** Show a leading status dot. Defaults to true. */
  dot?: boolean;
  /** `compact` matches compact table density. */
  size?: 'default' | 'compact';
}

/**
 * The one way to show a colored state (task result, DUT state, health) in the
 * Fleet Console. Do not hand-roll colored spans or Chips for status.
 */
export function StatusPill({
  tone,
  children,
  dot = true,
  size = 'default',
}: StatusPillProps) {
  const { fg, bg, dot: dotColor } = statusTones[tone];
  return (
    <Box
      component="span"
      data-testid="status-pill"
      data-tone={tone}
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '6px',
        height:
          size === 'compact' ? controlHeights.chipCompact : controlHeights.chip,
        px: 1.25,
        borderRadius: radii.pill,
        backgroundColor: bg,
        color: fg,
        fontFamily: fontFamilies.text,
        fontSize: fontSizes.caption,
        fontWeight: 500,
        lineHeight: 1,
        whiteSpace: 'nowrap',
        width: 'fit-content',
      }}
    >
      {dot && (
        <Box
          component="span"
          aria-hidden
          sx={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            backgroundColor: dotColor,
            flexShrink: 0,
          }}
        />
      )}
      {children}
    </Box>
  );
}

/** Renders a task result string as a `<StatusPill>`. */
export function TaskResultPill({ result }: { result: string | undefined }) {
  if (!result) return null;
  const { tone, label } = taskResultTone(result);
  return <StatusPill tone={tone}>{label}</StatusPill>;
}
