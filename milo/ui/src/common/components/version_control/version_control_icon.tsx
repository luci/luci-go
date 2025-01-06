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

import { Autorenew, History, Loop, Update } from '@mui/icons-material';
import { keyframes } from '@mui/material';

import { useIsSwitchingVersion } from './context';

const isNewUI = UI_VERSION_TYPE === 'new-ui';

const spin = keyframes`
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
`;

export function VersionControlIcon() {
  const isSwitching = useIsSwitchingVersion();

  if (isSwitching) {
    return isNewUI ? (
      <Loop sx={{ animation: `${spin} 1s infinite ease reverse` }} />
    ) : (
      <Autorenew sx={{ animation: `${spin} 1s infinite ease` }} />
    );
  }

  return isNewUI ? <History /> : <Update />;
}
