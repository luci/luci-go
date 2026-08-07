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

import { Grid } from '@mui/material';

import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { LOCATION_PATHS } from '../../utils/inventory_editing_utils';
import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';
import { CardForm } from '../form/CardForm';
import { FormAutocompleteField } from '../form/FormAutocompleteField';
import { FormTextField } from '../form/FormTextField';
import { useOptionalInventoryForm } from '../form/InventoryFormContext';

export interface PhysicalLocationCardProps {
  zone?: string | null;
  rack?: string | null;
  editable?: boolean;
  isEditing?: boolean;
  onEdit?: () => void;
}

export const PhysicalLocationCard = ({
  zone,
  rack,
  editable = false,
  isEditing = false,
  onEdit,
}: PhysicalLocationCardProps) => {
  const form = useOptionalInventoryForm();
  const dimensionsQuery = useDeviceDimensions({
    platform: Platform.CHROMEOS,
    enabled: Boolean(form),
  });

  const zoneOptions = dimensionsQuery.data?.labels?.['ufs_zone']?.values || [];

  const currentZone = form?.draftLse?.zone ?? zone;
  const currentRack = form?.draftLse?.rack ?? rack;
  const hasData = Boolean(currentZone || currentRack);

  if (form) {
    return (
      <CardForm
        cardId="location"
        title="Physical Location & Infrastructure"
        isEmpty={!hasData}
        emptyMessage="No physical location metadata recorded."
      >
        <Grid container spacing={2}>
          <FormAutocompleteField
            label="Zone"
            path={LOCATION_PATHS.zone}
            options={zoneOptions as string[]}
            multiple={false}
            gridSm={6}
          />
          <FormTextField label="Rack" path={LOCATION_PATHS.rack} gridSm={6} />
        </Grid>
      </CardForm>
    );
  }

  return (
    <InventoryDataCard
      title="Physical Location & Infrastructure"
      emptyMessage={
        !hasData ? 'No physical location metadata recorded.' : undefined
      }
      editable={editable}
      isEditing={isEditing}
      onEdit={onEdit}
    >
      <Grid container spacing={2}>
        <PropertyField label="Zone" value={zone} />
        <PropertyField label="Rack" value={rack} />
      </Grid>
    </InventoryDataCard>
  );
};
