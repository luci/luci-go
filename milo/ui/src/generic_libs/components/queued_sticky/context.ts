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

import { createContext, MutableRefObject } from 'react';

export const DepthCtx = createContext<number | undefined>(undefined);

export type Direction = 'top' | 'right' | 'bottom' | 'left';

export interface Offsets {
  readonly top: number;
  readonly right: number;
  readonly bottom: number;
  readonly left: number;
}

export const OffsetsCtx = createContext<Offsets | undefined>(undefined);

export interface SizeRecorder {
  recordSize(
    componentRef: MutableRefObject<undefined>,
    direction: Direction,
    size: number,
  ): void;

  remove(componentRef: MutableRefObject<undefined>): void;
}

export const SizeRecorderCtx = createContext<SizeRecorder | undefined>(
  undefined,
);
