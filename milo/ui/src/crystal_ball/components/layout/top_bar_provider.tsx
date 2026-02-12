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

import { ReactNode, useState } from 'react';

import { TopBarContext } from './top_bar_context';

/**
 * Provider for the TopBarContext.
 * Manages the state for the TopBar's title and actions.
 */
export function TopBarProvider({ children }: { children: ReactNode }) {
  const [title, setTitle] = useState<ReactNode | null>(null);
  const [actions, setActions] = useState<ReactNode | null>(null);

  return (
    <TopBarContext.Provider value={{ title, setTitle, actions, setActions }}>
      {children}
    </TopBarContext.Provider>
  );
}
