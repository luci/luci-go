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

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';

export const RunTargetColumnHeader = () => (
  <div
    css={{
      display: 'flex',
      alignItems: 'center',
      gap: '4px',
      maxWidth: '100%',
    }}
  >
    <Typography
      variant="subhead2"
      sx={{
        overflowX: 'hidden',
        textOverflow: 'ellipsis',
        fontWeight: 500,
      }}
    >
      Run Target
    </Typography>

    <InfoTooltip paperCss={{ maxWidth: '300px' }}>
      This inclues some fallbacks we use in case we dont get a{' '}
      <code>run_target</code> from omnilab. <br />
      In order:
      <ul css={{ marginTop: 0 }}>
        <li>
          <code>run_target</code>
        </li>
        <li>
          <code>product_board</code>
        </li>
        <li>
          <code>hardware</code>
        </li>
      </ul>
    </InfoTooltip>
  </div>
);
