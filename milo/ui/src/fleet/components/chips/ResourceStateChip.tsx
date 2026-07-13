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

import { Chip } from '@mui/material';

import { formatEnum } from '@/fleet/pages/device_details_page/chromeos/utils/formatters';
import { getStatusColor } from '@/fleet/pages/device_details_page/chromeos/utils/status_helpers';
import { colors } from '@/fleet/theme/colors';
import { stateToJSON } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/state.pb';

export interface ResourceStateChipProps {
  state: number | string | null | undefined;
}

export const ResourceStateChip = ({ state }: ResourceStateChipProps) => {
  const formatted = formatEnum(state, stateToJSON, 'STATE_');
  const label = formatted.replace(/_/g, ' ');

  const chipColor = getStatusColor(state);
  const variant = chipColor === colors.transparent ? 'outlined' : 'filled';

  return (
    <Chip
      label={label}
      size="small"
      variant={variant}
      sx={{
        backgroundColor:
          chipColor !== colors.transparent ? chipColor : undefined,
        fontWeight: 500,
        color:
          chipColor !== colors.transparent ? 'rgba(0, 0, 0, 0.87)' : undefined, // Light backgrounds expect dark text
      }}
    />
  );
};
