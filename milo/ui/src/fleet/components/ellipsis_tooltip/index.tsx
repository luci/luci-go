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

import { Tooltip } from '@mui/material';
import React, { useRef, useState } from 'react';

interface EllipsisTooltipProps {
  children: React.ReactNode;
  tooltip?: React.ReactNode;
}

function checkIsOverflowing(element: HTMLElement): boolean {
  if (element.scrollWidth > element.clientWidth) return true;

  for (let i = 0; i < element.children.length; i++) {
    const child = element.children[i] as HTMLElement;
    if (child && checkIsOverflowing(child)) return true;
  }

  return false;
}

export function EllipsisTooltip({ children, tooltip }: EllipsisTooltipProps) {
  const textRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);

  const handleOpen = () => {
    const el = textRef.current;
    if (el && checkIsOverflowing(el)) {
      setOpen(true);
    }
  };

  const handleClose = () => {
    setOpen(false);
  };

  return (
    <Tooltip
      title={tooltip ?? children}
      open={open}
      onOpen={handleOpen}
      onClose={handleClose}
      enterDelay={400}
    >
      <div
        ref={textRef}
        onMouseEnter={handleOpen}
        onMouseLeave={handleClose}
        onFocus={handleOpen}
        onBlur={handleClose}
        style={{
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          maxWidth: '100%',
          display: 'block',
        }}
      >
        {children}
      </div>
    </Tooltip>
  );
}
