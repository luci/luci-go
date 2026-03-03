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

import { Duration } from 'luxon';

import { toIsoString, toLuxonDateTime } from '@/fleet/utils/dates';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';
import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { fulfillmentStatusDisplayValueMap } from './fulfillment_status';

export interface DateWithOverdueData {
  value: string;
  overdue: Duration;
}

// RriGridRow describes the fields within a row in the UI.
export type RriGridRow = {
  id: string;
  rr_id: string;
  resource_request_bug_id: string;
  resource_details: string;
  resource_request_target_delivery_date: string;
  resource_request_actual_delivery_date: DateWithOverdueData;
  slippage: number;
  fulfillment_status: string;
  material_sourcing_actual_delivery_date: DateWithOverdueData;
  build_actual_delivery_date: DateWithOverdueData;
  qa_actual_delivery_date: DateWithOverdueData;
  config_actual_delivery_date: DateWithOverdueData;
  customer: string;
  resource_name: string;
  accepted_quantity: string;
  criticality: string;
  request_approval: string;
  resource_pm: string;
  fulfillment_channel: string;
  execution_status: string;
  resource_groups: string;
  resource_request_status: string;
  rr_bug_status: string;
};

export const getDateWithOverdueData = (
  actualDeliveryDate?: DateOnly,
  targetDeliveryDate?: DateOnly,
): DateWithOverdueData => {
  const overdue =
    actualDeliveryDate && targetDeliveryDate
      ? toLuxonDateTime(actualDeliveryDate)!.diff(
          toLuxonDateTime(targetDeliveryDate)!,
          'days',
        )
      : Duration.fromObject({ days: 0 });

  return {
    value: toIsoString(actualDeliveryDate),
    overdue: overdue,
  };
};

export const getRow = (rr: ResourceRequest): RriGridRow => {
  const resource_request_actual_delivery_date = getDateWithOverdueData(
    rr.resourceRequestActualDeliveryDate,
    rr.resourceRequestTargetDeliveryDate,
  );

  return {
    id: rr.rrId,
    rr_id: rr.rrId,
    resource_request_bug_id: rr.resourceRequestBugId ?? '',
    resource_details: rr.resourceDetails,
    resource_request_target_delivery_date: toIsoString(
      rr.resourceRequestTargetDeliveryDate,
    ),
    resource_request_actual_delivery_date,
    slippage:
      resource_request_actual_delivery_date.overdue.shiftTo('days').days,
    fulfillment_status:
      rr.fulfillmentStatus !== undefined
        ? fulfillmentStatusDisplayValueMap[
            ResourceRequest_Status[
              rr.fulfillmentStatus
            ] as keyof typeof ResourceRequest_Status
          ]
        : '',
    material_sourcing_actual_delivery_date: getDateWithOverdueData(
      rr.procurementActualDeliveryDate,
      rr.procurementTargetDeliveryDate,
    ),
    build_actual_delivery_date: getDateWithOverdueData(
      rr.buildActualDeliveryDate,
      rr.buildTargetDeliveryDate,
    ),
    qa_actual_delivery_date: getDateWithOverdueData(
      rr.qaActualDeliveryDate,
      rr.qaTargetDeliveryDate,
    ),
    config_actual_delivery_date: getDateWithOverdueData(
      rr.configActualDeliveryDate,
      rr.configTargetDeliveryDate,
    ),
    customer: rr.customer ?? '',
    resource_name: rr.resourceName ?? '',
    accepted_quantity: rr.acceptedQuantity?.toString() ?? '',
    criticality: rr.criticality ?? '',
    request_approval: rr.requestApproval ?? '',
    resource_pm: rr.resourcePm ?? '',
    fulfillment_channel: rr.fulfillmentChannel ?? '',
    execution_status: rr.executionStatus ?? '',
    resource_groups: rr.resourceGroups?.join(', ') ?? '',
    resource_request_status:
      rr.resourceRequestStatus !== undefined
        ? fulfillmentStatusDisplayValueMap[
            ResourceRequest_Status[
              rr.resourceRequestStatus
            ] as keyof typeof ResourceRequest_Status
          ]
        : '',
    rr_bug_status: rr.resourceRequestBugStatus ?? '',
  };
};
