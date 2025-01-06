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

import { useMutation } from '@tanstack/react-query';
import { ReactNode, useCallback, useRef, useState } from 'react';

import { IsSwitchingVersionCtx, SwitchVersionCtx } from './context';
import { switchToNewUI, switchToOldUI } from './controls';

const isNewUI = UI_VERSION_TYPE === 'new-ui';

export interface VersionControlProviderProps {
  readonly children: ReactNode;
}

export function VersionControlProvider({
  children,
}: VersionControlProviderProps) {
  const [switched, setSwitched] = useState(false);

  const uiVersionMutation = useMutation({
    mutationKey: ['ui-version', UI_VERSION_TYPE],
    mutationFn: async () => {
      setSwitched(true);
      if (isNewUI) {
        await switchToOldUI();
      } else {
        await switchToNewUI();
      }
    },
  });
  if (uiVersionMutation.isError) {
    throw uiVersionMutation.error;
  }

  const mutRef = useRef(uiVersionMutation);
  mutRef.current = uiVersionMutation;
  const switchVersion = useCallback(() => mutRef.current.mutate(), []);

  return (
    <SwitchVersionCtx.Provider value={switchVersion}>
      <IsSwitchingVersionCtx.Provider value={switched}>
        {children}
      </IsSwitchingVersionCtx.Provider>
    </SwitchVersionCtx.Provider>
  );
}
