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

import { Tooltip } from '@mui/material';
import _ from 'lodash';
import { ReactNode, useEffect, useRef, useState } from 'react';

export const EllipsisTooltip = ({
  children,
  tooltip,
}: {
  children: ReactNode;
  tooltip?: ReactNode;
}) => {
  const ref = useRef<HTMLDivElement>(null);
  const [isOverflowing, setIsOverflowing] = useState(false);

  // Usually the tooltip can automatically mange its own open state but when
  // using custom children components this breaks and we have to do it ourselves
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    setIsOverflowing(checkIsOverflowing(element));

    const observer = new ResizeObserver(() =>
      setIsOverflowing(checkIsOverflowing(element)),
    );
    observer.observe(element);

    return () => {
      observer.disconnect();
    };
  }, []);

  return (
    <Tooltip title={tooltip ?? children} open={isOverflowing && open}>
      <div
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        ref={ref}
        css={{
          overflowX: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {children}
      </div>
    </Tooltip>
  );
};

// Checks if the element (or his only child) is overflowing
const checkIsOverflowing = (element: HTMLDivElement) => {
  if (element.scrollWidth > element.clientWidth) return true;

  if (element.childNodes.length === 1) {
    if (!element.firstElementChild) return false;

    return (
      element.firstElementChild?.scrollWidth >
      element.firstElementChild?.clientWidth
    );
  }

  return false;
};
