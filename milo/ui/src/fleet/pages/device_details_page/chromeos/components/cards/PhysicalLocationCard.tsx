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

import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';

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
  const hasData = Boolean(zone || rack);

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
