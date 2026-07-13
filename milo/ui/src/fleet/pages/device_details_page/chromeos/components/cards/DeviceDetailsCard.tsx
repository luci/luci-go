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

import { Box, Grid, Typography } from '@mui/material';

import { ResourceStateChip } from '@/fleet/components/chips/ResourceStateChip';

import { safeFormatDate } from '../../utils/formatters';
import { CodeChip } from '../common/CodeChip';
import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';

export interface DeviceDetailsInfo {
  hostname?: string | null;
  name?: string | null;
  description?: string | null;
  machines?: readonly string[] | string[];
  resourceState?: number | string | null;
  machineLsePrototype?: string | null;
  realm?: string | null;
  deploymentTicket?: string | null;
  updateTime?: string | null;
}

export interface DeviceDetailsCardProps {
  data: DeviceDetailsInfo | null | undefined;
  editable?: boolean;
  isEditing?: boolean;
  onEdit?: () => void;
}

export const DeviceDetailsCard = ({
  data,
  editable = false,
  isEditing = false,
  onEdit,
}: DeviceDetailsCardProps) => {
  const hostname = data?.hostname;
  const name = data?.name;
  const description = data?.description;
  const machines = data?.machines || [];
  const resourceState = data?.resourceState;
  const machineLsePrototype = data?.machineLsePrototype;
  const realm = data?.realm;
  const deploymentTicket = data?.deploymentTicket;
  const updateTime = data?.updateTime;
  const hasData = Boolean(
    hostname ||
      name ||
      description ||
      machines.length > 0 ||
      resourceState ||
      machineLsePrototype ||
      realm ||
      deploymentTicket ||
      updateTime,
  );

  return (
    <InventoryDataCard
      title="Device Details"
      emptyMessage={!hasData ? 'No device details configured.' : undefined}
      editable={editable}
      isEditing={isEditing}
      onEdit={onEdit}
    >
      <Grid container spacing={2}>
        <PropertyField label="Hostname" value={hostname}>
          {hostname ? (
            <CodeChip value={hostname} />
          ) : (
            <Typography variant="body2">N/A</Typography>
          )}
        </PropertyField>

        <PropertyField label="UFS Resource Name" value={name} />

        <PropertyField label="Resource State" value={resourceState}>
          <ResourceStateChip state={resourceState} />
        </PropertyField>

        {machines.length > 0 && (
          <PropertyField label="Associated Asset IDs" value="yes">
            <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
              {machines.map((m) => (
                <CodeChip key={m} value={m} />
              ))}
            </Box>
          </PropertyField>
        )}

        <PropertyField label="Security Realm" value={realm} variant="text" />

        <PropertyField
          label="Deployment Ticket"
          value={deploymentTicket}
          variant="text"
        />

        <PropertyField
          label="MachineLSE Prototype"
          value={machineLsePrototype}
        />

        {Boolean(updateTime?.trim()) && (
          <PropertyField label="Last Modified in UFS" value="yes">
            <Typography variant="body2">
              {safeFormatDate(updateTime)}
            </Typography>
          </PropertyField>
        )}

        <PropertyField
          label="Description"
          value={description}
          variant="text"
          gridSm={12}
        />
      </Grid>
    </InventoryDataCard>
  );
};
