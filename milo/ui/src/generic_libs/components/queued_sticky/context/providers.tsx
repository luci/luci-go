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

import { MutableRefObject, ReactNode, useMemo, useRef, useState } from 'react';

import { DepthCtx, Direction, OffsetsCtx, SizeRecorderCtx } from './context';
import { useDepth } from './hooks';

export interface QueuedStickyContextProviderProps {
  readonly children: ReactNode;
}

export function QueuedStickyContextProvider({
  children,
}: QueuedStickyContextProviderProps) {
  const parentDepth = useDepth();
  // Maps hook ref to sizes.
  const topStickies = useRef(new Map<MutableRefObject<undefined>, number>());
  const rightStickies = useRef(new Map<MutableRefObject<undefined>, number>());
  const bottomStickies = useRef(new Map<MutableRefObject<undefined>, number>());
  const leftStickies = useRef(new Map<MutableRefObject<undefined>, number>());

  const [offsets, setOffsets] = useState({
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
  });
  const updateSize = useMemo(() => {
    const stickies = {
      top: topStickies.current,
      right: rightStickies.current,
      bottom: bottomStickies.current,
      left: leftStickies.current,
    };
    return {
      recordSize(
        componentRef: MutableRefObject<undefined>,
        direction: Direction,
        size: number,
      ) {
        const directionStickies = stickies[direction];
        if (size === null) {
          directionStickies.delete(componentRef);
        } else {
          directionStickies.set(componentRef, size);
        }
        setOffsets((prev) => {
          const updated = Math.max(...directionStickies.values());
          if (prev[direction] === updated) {
            return prev;
          }
          return {
            ...prev,
            [direction]: updated,
          };
        });
      },
      remove(componentRef: MutableRefObject<undefined>) {
        for (const [direction, directionStickies] of Object.entries(stickies)) {
          directionStickies.delete(componentRef);
          setOffsets((prev) => {
            const updated = Math.max(...directionStickies.values());
            if (prev[direction as Direction] === updated) {
              return prev;
            }
            return {
              ...prev,
              [direction]: updated,
            };
          });
        }
      },
    };
  }, []);

  return (
    <DepthCtx.Provider value={parentDepth + 1}>
      <SizeRecorderCtx.Provider value={updateSize}>
        <OffsetsCtx.Provider value={offsets}>{children}</OffsetsCtx.Provider>
      </SizeRecorderCtx.Provider>
    </DepthCtx.Provider>
  );
}
