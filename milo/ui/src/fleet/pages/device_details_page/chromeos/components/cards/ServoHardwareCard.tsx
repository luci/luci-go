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
import { useMemo } from 'react';

import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { SERVO_PATHS } from '../../utils/inventory_editing_utils';
import { CardForm } from '../form/CardForm';
import { FormAutocompleteField } from '../form/FormAutocompleteField';
import { FormTextField } from '../form/FormTextField';
import { useInventoryForm } from '../form/InventoryFormContext';

export interface ServoInfo {
  servoHostname?: string | null;
  servoPort?: number | string | null;
  servoSerial?: string | null;
}

export const ServoHardwareCard = () => {
  const { draftLse } = useInventoryForm();

  const servo =
    draftLse?.chromeosMachineLse?.deviceLse?.dut?.peripherals?.servo;

  const hasServoData = Boolean(
    servo &&
      (servo.servoHostname ||
        servo.servoSerial ||
        (servo.servoPort !== undefined &&
          servo.servoPort !== null &&
          Number(servo.servoPort) > 0)),
  );

  const dimensionsQuery = useDeviceDimensions({ platform: Platform.CHROMEOS });
  const hostnameOptions = useMemo(() => {
    if (!dimensionsQuery.data) return [];
    const names: string[] = [];
    const baseDutName = dimensionsQuery.data.baseDimensions?.['dut_name'];
    const labelDutName = dimensionsQuery.data.labels?.['dut_name'];
    const labelAssoc =
      dimensionsQuery.data.labels?.['label-associated_hostname'];
    if (baseDutName?.values) names.push(...baseDutName.values);
    if (labelDutName?.values) names.push(...labelDutName.values);
    if (labelAssoc?.values) names.push(...labelAssoc.values);
    return Array.from(new Set(names)).sort();
  }, [dimensionsQuery.data]);

  return (
    <CardForm
      cardId="servo"
      title="Servo"
      isEmpty={!hasServoData}
      emptyMessage="No Servo debugging hardware attached."
    >
      <Grid container spacing={2}>
        <FormAutocompleteField
          label="Hostname"
          path={SERVO_PATHS.hostname}
          options={hostnameOptions}
          multiple={false}
          gridSm={6}
        />
        <FormTextField label="Serial" path={SERVO_PATHS.serial} gridSm={6} />
        <FormTextField
          label="Port"
          path={SERVO_PATHS.port}
          type="number"
          gridSm={6}
        />
      </Grid>
    </CardForm>
  );
};
