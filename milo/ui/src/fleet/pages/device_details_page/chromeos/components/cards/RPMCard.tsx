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

import { Chip, Grid } from '@mui/material';

import { oSRPM_TypeToJSON } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/chromeos/lab/rpm.pb';

import { formatEnum } from '../../utils/formatters';
import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';

export interface RPMInfo {
  powerunitName?: string | null;
  powerunitOutlet?: string | null;
  powerunitType?: number | string | null;
}

export interface RPMCardProps {
  rpm?: RPMInfo | null;
  editable?: boolean;
  isEditing?: boolean;
  onEdit?: () => void;
}

const hasRpmInfo = (rpm?: RPMInfo | null) =>
  Boolean(
    rpm &&
      (rpm.powerunitName ||
        rpm.powerunitOutlet ||
        (rpm.powerunitType !== undefined && rpm.powerunitType !== null)),
  );

export const RPMCard = ({
  rpm,
  editable = false,
  isEditing = false,
  onEdit,
}: RPMCardProps) => {
  const hasDutRpm = hasRpmInfo(rpm);

  return (
    <InventoryDataCard
      title="RPM"
      emptyMessage={
        !hasDutRpm
          ? 'No RPM outlets or power distribution units configured.'
          : undefined
      }
      editable={editable && Boolean(onEdit)}
      isEditing={isEditing}
      onEdit={onEdit}
    >
      <Grid container spacing={2}>
        {hasDutRpm && rpm && (
          <Grid item xs={12}>
            <Grid container spacing={2}>
              <PropertyField
                label="Name"
                value={rpm.powerunitName}
                gridSm={6}
              />

              <PropertyField
                label="Outlet"
                value={rpm.powerunitOutlet}
                variant="text"
                gridSm={3}
              />

              {rpm.powerunitType !== undefined &&
                rpm.powerunitType !== null && (
                  <PropertyField label="Type" gridSm={3}>
                    <Chip
                      label={formatEnum(
                        rpm.powerunitType,
                        oSRPM_TypeToJSON,
                        'TYPE_',
                      )}
                      size="small"
                      variant="outlined"
                    />
                  </PropertyField>
                )}
            </Grid>
          </Grid>
        )}
      </Grid>
    </InventoryDataCard>
  );
};
