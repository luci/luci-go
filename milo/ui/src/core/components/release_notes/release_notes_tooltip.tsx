// Copyright 2023 The LUCI Authors.
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

import {
  Button,
  Tooltip,
  TooltipProps,
  Typography,
  styled,
  tooltipClasses,
} from '@mui/material';
import { forwardRef, useMemo, useRef } from 'react';
import { Link } from 'react-router-dom';
import { useClickAway, useTimeout } from 'react-use';

import { SanitizedHtml } from '@/common/components/sanitized_html';

import { renderReleaseNotes } from './common';
import {
  useReleaseNotes,
  useHasNewRelease,
  useMarkReleaseNotesRead,
} from './context';

const TooltipDisplay = styled(
  forwardRef(function TooltipDisplay(
    { className, ...props }: TooltipProps,
    ref,
  ) {
    return <Tooltip {...props} classes={{ popper: className }} ref={ref} />;
  }),
)(({ theme }) => ({
  [`& .${tooltipClasses.tooltip}`]: {
    backgroundColor: '#f5f5f9',
    color: 'rgba(0, 0, 0, 0.87)',
    maxWidth: 300,
    fontSize: theme.typography.pxToRem(12),
    border: '1px solid #dadde9',
  },
  [`& .${tooltipClasses.arrow}::before`]: {
    backgroundColor: '#f5f5f9',
    color: 'rgba(0, 0, 0, 0.87)',
    border: '1px solid #dadde9',
  },
}));

export interface ReleaseNotesTooltipProps {
  readonly children: JSX.Element;
}

export function ReleaseNotesTooltip({ children }: ReleaseNotesTooltipProps) {
  const releaseNotes = useReleaseNotes();
  const hasNewRelease = useHasNewRelease();
  const markAsRead = useMarkReleaseNotesRead();
  const releaseNotesHtml = useMemo(() => {
    const latestWithoutDates = releaseNotes.latest.replace(
      /(?<=^|\n)## \d{4}-\d{2}-\d{2}\n/g,
      '\n',
    );
    return renderReleaseNotes(latestWithoutDates);
  }, [releaseNotes.latest]);

  const tooltipRef = useRef<HTMLDivElement>(null);
  const [stayedLongEnough] = useTimeout(5000);
  useClickAway(tooltipRef, () => {
    if (stayedLongEnough()) {
      markAsRead();
    }
  });

  return (
    <TooltipDisplay
      title={
        <div ref={tooltipRef}>
          <Typography variant="h6">{"What's new?"}</Typography>
          <SanitizedHtml
            html={releaseNotesHtml}
            sx={{ '& ul': { margin: '5px' } }}
          />
          <div css={{ display: 'grid', gridTemplateColumns: '1fr auto' }}>
            <div css={{ paddingTop: '9px' }}>
              <Link to="/ui/doc/release-notes">Full release notes</Link>
            </div>
            <Button onClick={() => markAsRead()}>Dismiss</Button>
          </div>
        </div>
      }
      arrow
      open={hasNewRelease}
    >
      {/* Elements under a MUI tooltip shall not have its own tooltip/title. But
       ** we use it as a notification box rather than a regular tooltip. Add an
       ** extra div to silence the warning in case the child has a tooltip. */}
      <div>{children}</div>
    </TooltipDisplay>
  );
}
