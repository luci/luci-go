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

import { Box, Chip, Grid } from '@mui/material';

import { logicalZoneToJSON } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import { formatEnum } from '../../utils/formatters';
import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';

export interface LogicalSchedulingCardProps {
  pools?: string[];
  logicalZone?: number | string | null;
  hive?: string | null;
  editable?: boolean;
  isEditing?: boolean;
  onEdit?: () => void;
}

export const LogicalSchedulingCard = ({
  pools = [],
  logicalZone,
  hive,
  editable = false,
  isEditing = false,
  onEdit,
}: LogicalSchedulingCardProps) => {
  const logicalZoneLabel = formatEnum(
    logicalZone,
    logicalZoneToJSON,
    'LOGICAL_ZONE_',
  );
  const hasLogicalZone =
    logicalZoneLabel !== 'UNSPECIFIED' &&
    logicalZoneLabel !== '0' &&
    logicalZoneLabel !== 'N/A' &&
    logicalZoneLabel !== '';

  const hasData = Boolean(pools.length || hasLogicalZone || hive);

  return (
    <InventoryDataCard
      title="Pools & Task Routing"
      emptyMessage={
        !hasData ? 'No scheduling tags or pool labels assigned.' : undefined
      }
      editable={editable}
      isEditing={isEditing}
      onEdit={onEdit}
    >
      <Grid container spacing={2}>
        {pools.length > 0 && (
          <PropertyField label="Pools" gridSm={12}>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75 }}>
              {pools.map((pool, index) => (
                <Chip
                  key={`${pool}-${index}`}
                  label={pool}
                  size="small"
                  color="primary"
                  variant="outlined"
                  sx={{ fontWeight: 600 }}
                />
              ))}
            </Box>
          </PropertyField>
        )}

        <PropertyField
          label="Logical Zone"
          value={hasLogicalZone ? logicalZoneLabel : null}
        />

        <PropertyField label="Hive" value={hive} />
      </Grid>
    </InventoryDataCard>
  );
};
