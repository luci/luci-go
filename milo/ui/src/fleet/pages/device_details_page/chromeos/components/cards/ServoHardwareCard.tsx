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

export interface ServoInfo {
  servoHostname?: string | null;
  servoPort?: number | string | null;
  servoSerial?: string | null;
}

export interface ServoHardwareCardProps {
  servo?: ServoInfo | null;
  editable?: boolean;
  isEditing?: boolean;
  onEdit?: () => void;
}

export const ServoHardwareCard = ({
  servo,
  editable = false,
  isEditing = false,
  onEdit,
}: ServoHardwareCardProps) => {
  const hasServoData = Boolean(
    servo &&
      (servo.servoHostname ||
        servo.servoSerial ||
        (servo.servoPort !== undefined &&
          servo.servoPort !== null &&
          Number(servo.servoPort) > 0)),
  );

  return (
    <InventoryDataCard
      title="Servo"
      emptyMessage={
        !hasServoData ? 'No Servo debugging hardware attached.' : undefined
      }
      editable={editable}
      isEditing={isEditing}
      onEdit={onEdit}
    >
      <Grid container spacing={2}>
        <PropertyField label="Hostname" value={servo?.servoHostname} />
        <PropertyField label="Serial" value={servo?.servoSerial} />
        <PropertyField
          label="Port"
          value={
            servo?.servoPort !== undefined &&
            servo?.servoPort !== null &&
            Number(servo.servoPort) > 0
              ? String(servo.servoPort)
              : null
          }
          variant="text"
        />
      </Grid>
    </InventoryDataCard>
  );
};
