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
import { useMemo } from 'react';

import {
  OSRPM_Type,
  oSRPM_TypeFromJSON,
  oSRPM_TypeToJSON,
} from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/chromeos/lab/rpm.pb';

import { formatEnum } from '../../utils/formatters';
import { RPM_PATHS } from '../../utils/inventory_editing_utils';
import { InventoryDataCard } from '../common/InventoryDataCard';
import { PropertyField } from '../common/PropertyField';
import { CardForm } from '../form/CardForm';
import { FormAutocompleteField } from '../form/FormAutocompleteField';
import { FormTextField } from '../form/FormTextField';
import { useOptionalInventoryForm } from '../form/InventoryFormContext';

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
  const form = useOptionalInventoryForm();
  const isLabstation = Boolean(
    form?.draftLse?.chromeosMachineLse?.deviceLse?.labstation,
  );

  const cardId = 'rpm';
  const isEditingMode = form ? form.activeEditingCardId === cardId : isEditing;
  const isEditable = form ? form.editable : editable;

  const draftRpm = isLabstation
    ? form?.draftLse?.chromeosMachineLse?.deviceLse?.labstation?.rpm
    : form?.draftLse?.chromeosMachineLse?.deviceLse?.dut?.peripherals?.rpm;

  const displayRpm = form ? draftRpm : rpm;
  const hasDutRpm = hasRpmInfo(displayRpm);

  const paths = isLabstation ? RPM_PATHS.labstation : RPM_PATHS.dut;

  const rpmTypeOptions = useMemo(
    () =>
      Object.keys(OSRPM_Type)
        .filter(
          (key) =>
            isNaN(Number(key)) &&
            key !== 'TYPE_UNKNOWN' &&
            key !== 'UNRECOGNIZED',
        )
        .map((key) => key.replace('TYPE_', '')),
    [],
  );

  const rpmTypeValueMapping = useMemo(
    () => ({
      toDisplay: (val: unknown) =>
        val ? formatEnum(val as number, oSRPM_TypeToJSON, 'TYPE_') : '',
      toStored: (val: string) => (val ? oSRPM_TypeFromJSON(`TYPE_${val}`) : 0),
    }),
    [],
  );

  const content = (
    <Grid container spacing={2}>
      {isEditingMode ? (
        <>
          <FormTextField label="Name" path={paths.host} gridSm={6} />
          <FormTextField label="Outlet" path={paths.outlet} gridSm={3} />
          <FormAutocompleteField
            label="Type"
            path={paths.type}
            options={rpmTypeOptions}
            valueMapping={rpmTypeValueMapping}
            multiple={false}
            freeSolo={false}
            gridSm={3}
          />
        </>
      ) : (
        hasDutRpm &&
        displayRpm && (
          <Grid item xs={12}>
            <Grid container spacing={2}>
              <PropertyField
                label="Name"
                value={displayRpm.powerunitName}
                gridSm={6}
              />

              <PropertyField
                label="Outlet"
                value={displayRpm.powerunitOutlet}
                variant="text"
                gridSm={3}
              />

              {displayRpm.powerunitType !== undefined &&
                displayRpm.powerunitType !== null && (
                  <PropertyField label="Type" gridSm={3}>
                    <Chip
                      label={formatEnum(
                        displayRpm.powerunitType,
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
        )
      )}
    </Grid>
  );

  if (form) {
    return (
      <CardForm
        cardId={cardId}
        title="RPM"
        isEmpty={!hasDutRpm}
        emptyMessage="No RPM outlets or power distribution units configured."
      >
        {content}
      </CardForm>
    );
  }

  return (
    <InventoryDataCard
      title="RPM"
      emptyMessage={
        !hasDutRpm
          ? 'No RPM outlets or power distribution units configured.'
          : undefined
      }
      editable={isEditable && Boolean(onEdit)}
      isEditing={isEditing}
      onEdit={onEdit}
    >
      {content}
    </InventoryDataCard>
  );
};
