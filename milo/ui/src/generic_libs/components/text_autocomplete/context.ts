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

import { createContext, useContext } from 'react';

export interface SettersContext {
  readonly clear: () => void;
  readonly commit: () => void;
}

export const SettersCtx = createContext<SettersContext | undefined>(undefined);

export function useSetters() {
  const ctx = useContext(SettersCtx);
  if (ctx === undefined) {
    throw new Error('useSetters can only be used in a TextAutocomplete');
  }
  return ctx;
}

export interface InputStateContext {
  readonly hasUncommitted: boolean;
  readonly isEmpty: boolean;
}

export const InputStateCtx = createContext<InputStateContext | undefined>(
  undefined,
);

export function useInputState() {
  const ctx = useContext(InputStateCtx);
  if (ctx === undefined) {
    throw new Error('useInputState can only be used in a TextAutocomplete');
  }
  return ctx;
}
