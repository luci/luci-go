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

import { Grid, Link, Typography } from '@mui/material';

import { ResourceStateChip } from '@/fleet/components/chips/ResourceStateChip';

import { safeFormatDate } from '../../utils/formatters';
import { CodeChip } from '../common/CodeChip';
import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';

export interface DeviceDetailsInfo {
  hostname?: string | null;
  description?: string | null;
  resourceState?: number | string | null;
  machineLsePrototype?: string | null;
  realm?: string | null;
  deploymentTicket?: string | null;
  updateTime?: string | null;
  board?: string | null;
  model?: string | null;
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
  const description = data?.description;
  const resourceState = data?.resourceState;
  const machineLsePrototype = data?.machineLsePrototype;
  const realm = data?.realm;
  const deploymentTicket = data?.deploymentTicket;
  const updateTime = data?.updateTime;
  const board = data?.board;
  const model = data?.model;
  const hasData = Boolean(
    hostname ||
      description ||
      resourceState ||
      machineLsePrototype ||
      realm ||
      deploymentTicket ||
      updateTime ||
      board ||
      model,
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

        <PropertyField label="Board" value={board}>
          {board ? (
            <Link
              href={`https://go/dlm-board/${encodeURIComponent(board)}`}
              target="_blank"
              rel="noreferrer"
              sx={{ textDecoration: 'none' }}
            >
              <CodeChip value={board} />
            </Link>
          ) : (
            <Typography variant="body2">N/A</Typography>
          )}
        </PropertyField>

        <PropertyField label="Resource State" value={resourceState}>
          <ResourceStateChip state={resourceState} />
        </PropertyField>

        <PropertyField label="Model" value={model}>
          {model ? (
            <Link
              href={`https://go/dlm-model/${encodeURIComponent(model)}`}
              target="_blank"
              rel="noreferrer"
              sx={{ textDecoration: 'none' }}
            >
              <CodeChip value={model} />
            </Link>
          ) : (
            <Typography variant="body2">N/A</Typography>
          )}
        </PropertyField>

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
