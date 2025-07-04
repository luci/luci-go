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

import { Typography } from '@mui/material';
import _ from 'lodash';

import sadBass from '@/fleet/assets/pngs/sad_bass.png';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { platformRenderString } from './platform_selector';

export const PlatformNotAvailable = ({
  availablePlatforms,
}: {
  availablePlatforms?: Platform[];
}) => {
  return (
    <div
      css={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexDirection: 'column',
        margin: 50,
      }}
    >
      <img
        alt="logo"
        id="luci-icon"
        src={sadBass}
        css={{
          width: '50%',
          maxWidth: 400,
        }}
      />
      <Typography variant="h1" sx={{ fontSize: 50, textAlign: 'center' }}>
        Platform not yet available
      </Typography>

      {availablePlatforms && availablePlatforms.length > 0 && (
        <Typography variant="body1" sx={{ marginTop: '30px' }}>
          try{' '}
          {availablePlatforms
            .map(platformRenderString)
            .map(_.lowerCase)
            .join(', ')}
        </Typography>
      )}
    </div>
  );
};
