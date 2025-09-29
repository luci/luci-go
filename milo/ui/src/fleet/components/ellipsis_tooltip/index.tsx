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
  const ref = useRef<HTMLSpanElement>(null);
  const [isOverflowing, setIsOverflowing] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    setIsOverflowing(element.scrollWidth > element.clientWidth);

    const observer = new ResizeObserver(() =>
      setIsOverflowing(element.scrollWidth > element.clientWidth),
    );
    observer.observe(element);

    return () => {
      observer.disconnect();
    };
  }, []);

  return (
    <Tooltip title={tooltip ?? children} disableHoverListener={!isOverflowing}>
      <span
        ref={ref}
        style={{
          whiteSpace: 'nowrap',
          overflowX: 'hidden',
          textOverflow: 'ellipsis',
        }}
      >
        {children}
      </span>
    </Tooltip>
  );
};
