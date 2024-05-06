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

import { Box } from '@mui/material';
import { ReactNode, SVGProps, useEffect, useRef, useState } from 'react';

export interface ContainedForeignObjectProps
  extends Omit<SVGProps<SVGForeignObjectElement>, 'width' | 'height'> {
  /**
   * Reacting to children size change is not supported.
   * Adding this mandatory property to make it clear.
   */
  readonly fixedSizeChildren: true;
  readonly children: ReactNode;
}

/**
 * A <foreignObject /> that sets its height and width to match its children.
 */
export function ContainedForeignObject({
  fixedSizeChildren: _fixedSizeChildren,
  children,
  ...props
}: ContainedForeignObjectProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState<readonly [number, number]>([0, 0]);

  useEffect(() => {
    // This should never happen. Useful for type narrowing.
    if (!containerRef.current) {
      return;
    }
    const rect = containerRef.current.getBoundingClientRect();
    setSize([rect.width, rect.height]);
  }, []);

  return (
    <foreignObject {...props} width={size[0]} height={size[1]}>
      <Box sx={{ display: 'inline-block' }} ref={containerRef}>
        {children}
      </Box>
    </foreignObject>
  );
}
