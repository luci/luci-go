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

import { Chip, Typography } from '@mui/material';
import { Box } from '@mui/system';
import _ from 'lodash';

import sadBass from '@/fleet/assets/pngs/sad_bass.png';
import { usePlatform } from '@/fleet/hooks/usePlatform';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { platformRenderString } from '../hooks/usePlatform';

export const PlatformNotAvailable = ({
  availablePlatforms,
}: {
  availablePlatforms?: Platform[];
}) => {
  const platform = usePlatform();
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
        <Box
          sx={{
            display: 'flex',
            gap: '10px',
            marginTop: '30px',
            alignItems: 'center',
          }}
        >
          <Typography variant="body1">Try</Typography>
          {availablePlatforms.map((p) => (
            <Chip
              key={p}
              label={_.lowerCase(platformRenderString(p))}
              onClick={() => platform.setPlatform(p)}
              variant="outlined"
              sx={{
                fontSize: '1rem',
              }}
            />
          ))}
        </Box>
      )}
    </div>
  );
};
