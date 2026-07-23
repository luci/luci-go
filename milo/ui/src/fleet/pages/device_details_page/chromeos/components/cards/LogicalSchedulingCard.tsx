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

import { logicalZoneToJSON } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import { formatEnum } from '../../utils/formatters';
import {
  getNestedValue,
  LOGICAL_SCHEDULING_PATHS,
} from '../../utils/inventory_editing_utils';
import { PropertyField } from '../common/PropertyField';
import { CardForm } from '../form/CardForm';
import { FormTextField } from '../form/FormTextField';
import { useInventoryForm } from '../form/InventoryFormContext';

export const LogicalSchedulingCard = () => {
  const { draftLse } = useInventoryForm();

  const isLabstation = Boolean(
    draftLse?.chromeosMachineLse?.deviceLse?.labstation,
  );
  const poolsPath = isLabstation
    ? LOGICAL_SCHEDULING_PATHS.labstationPools
    : LOGICAL_SCHEDULING_PATHS.dutPools;

  const pools = (getNestedValue(draftLse, poolsPath) as string[]) || [];

  const logicalZone = draftLse?.logicalZone;
  const hive =
    draftLse?.chromeosMachineLse?.deviceLse?.dut?.hive ||
    draftLse?.chromeosMachineLse?.deviceLse?.labstation?.hive;

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
    <CardForm
      cardId="logical"
      title="Pools & Task Routing"
      isEmpty={!hasData}
      emptyMessage="No scheduling tags or pool labels assigned."
    >
      <Grid container spacing={2}>
        <FormTextField
          label="Pools"
          path={poolsPath}
          type="array"
          gridSm={12}
        />
        <PropertyField
          label="Logical Zone"
          value={hasLogicalZone ? logicalZoneLabel : null}
        />
        <PropertyField label="Hive" value={hive} />
      </Grid>
    </CardForm>
  );
};
