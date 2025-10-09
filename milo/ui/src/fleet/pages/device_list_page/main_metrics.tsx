// Copyright 2024 The LUCI Authors.
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

import { type CSSObject } from '@emotion/react';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Typography } from '@mui/material';
import { useQuery, UseQueryResult } from '@tanstack/react-query';

import { LeaseStateInfo } from '@/fleet/components/lease_state_info/lease_state_info';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  CountDevicesResponse,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import {
  addNewFilterToParams,
  stringifyFilters,
} from '../../components/filter_dropdown/search_param_utils/search_param_utils';
import { SingleMetric } from '../../components/summary_header/single_metric';

const HAS_RIGHT_SIBLING_STYLES: CSSObject = {
  borderRight: `1px solid ${colors.grey[300]}`,
  marginRight: 16,
  paddingRight: 8,
};

const METRIC_CONTAINER_STYLES: CSSObject = {
  display: 'flex',
  justifyContent: 'flex-start',
  marginTop: 4,
  flexWrap: 'wrap',
  gap: 8,
};

// Recommended filters for finding all CrOS labstations. See: b/398911822#comment4
const LABSTATION_FILTERS: SelectedOptions = {
  'labels.label-pool': [
    'labstation_tryjob',
    'labstation_main',
    'labstation_canary',
  ],
};

interface MainMetricsProps {
  countQuery: UseQueryResult<CountDevicesResponse, unknown>;
  labstationsQuery: UseQueryResult<CountDevicesResponse, unknown>;
}

/**
 * MainMetrics is the presentational component for the main metrics section.
 * It is responsible for displaying the metrics, but not for fetching the data.
 */
export function ChromeOSMainMetrics({
  countQuery,
  labstationsQuery,
}: MainMetricsProps) {
  const [searchParams, _] = useSyncedSearchParams();

  const getFilterQueryString = (filters: Record<string, string[]>): string => {
    let newSearchParams = new URLSearchParams(searchParams);
    for (const [name, values] of Object.entries(filters)) {
      newSearchParams = addNewFilterToParams(newSearchParams, name, values);
    }
    return '?' + newSearchParams.toString();
  };

  const getContent = () => {
    if (countQuery.isError || labstationsQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(
            countQuery.error || labstationsQuery.error,
            'get the main metrics',
          )}
        </Alert>
      );
    }

    return (
      <div
        css={{
          display: 'flex',
          maxWidth: 1400,
        }}
      >
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.2,
          }}
        >
          <Typography variant="subhead1">Total</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Devices"
              value={countQuery.data?.total}
              loading={countQuery.isPending}
            />
          </div>
        </div>
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.3,
          }}
        >
          <div css={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Typography variant="subhead1">Lease state</Typography>
            <LeaseStateInfo />
          </div>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Leased"
              value={countQuery.data?.taskState?.busy}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['DEVICE_STATE_LEASED'],
              })}
            />
            <SingleMetric
              name="Available"
              value={countQuery.data?.taskState?.idle}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['DEVICE_STATE_AVAILABLE'],
              })}
            />
          </div>
        </div>
        <div css={{ ...HAS_RIGHT_SIBLING_STYLES, flexGrow: 2 }}>
          <Typography variant="subhead1">Device state</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Ready"
              value={countQuery.data?.deviceState?.ready}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['ready'], // TODO: Hotfix for b/449956551, needs further investigation on quote handling
              })}
            />
            <SingleMetric
              name="Need repair"
              value={countQuery.data?.deviceState?.needRepair}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['needs_repair'],
              })}
            />
            <SingleMetric
              name="Repair failed"
              value={countQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['repair_failed'],
              })}
            />
            <SingleMetric
              name="Needs deploy"
              value={countQuery.data?.deviceState?.needsDeploy}
              total={countQuery.data?.total}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['needs_deploy'],
              })}
            />
            <SingleMetric
              name="Needs replacement"
              value={countQuery.data?.deviceState?.needsReplacement}
              total={countQuery.data?.total}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['needs_replacement'],
              })}
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['needs_manual_repair'],
              })}
            />
          </div>
        </div>
        <div css={{ flexGrow: 1.5 }}>
          <Typography variant="subhead1">Labstation state</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Ready"
              value={labstationsQuery.data?.deviceState?.ready}
              total={countQuery.data?.total}
              loading={labstationsQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['ready'],
                ...LABSTATION_FILTERS,
              })}
            />
            <SingleMetric
              name="Repair failed"
              value={labstationsQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              loading={labstationsQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['repair_failed'],
                ...LABSTATION_FILTERS,
              })}
            />
            <SingleMetric
              name="Needs deploy"
              value={labstationsQuery.data?.deviceState?.needsDeploy}
              total={countQuery.data?.total}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              loading={labstationsQuery.isPending}
              filterUrl={getFilterQueryString({
                'labels."dut_state"': ['needs_deploy'],
                ...LABSTATION_FILTERS,
              })}
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <MetricsContainer>
      <Typography variant="h4">Main metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </MetricsContainer>
  );
}

export function AndroidMainMetrics({
  countQuery,
}: {
  countQuery: UseQueryResult<CountDevicesResponse, unknown>;
}) {
  const [searchParams, _] = useSyncedSearchParams();

  const getFilterQueryString = (filters: Record<string, string[]>): string => {
    let newSearchParams = new URLSearchParams(searchParams);
    for (const [name, values] of Object.entries(filters)) {
      newSearchParams = addNewFilterToParams(newSearchParams, name, values);
    }
    return '?' + newSearchParams.toString();
  };

  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(countQuery.error, 'get the main metrics')}
        </Alert>
      );
    }

    return (
      <div
        css={{
          display: 'flex',
          maxWidth: 1400,
        }}
      >
        <div css={{ flexGrow: 2 }}>
          <Typography variant="subhead1">Device state</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Total"
              value={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Ready"
              value={countQuery.data?.androidCount?.idleDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['IDLE'],
              })}
            />
            <SingleMetric
              name="Busy"
              value={countQuery.data?.androidCount?.busyDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['BUSY'],
              })}
            />
            <SingleMetric
              name="Prepping"
              value={countQuery.data?.androidCount?.preppingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['PREPPING'],
              })}
            />
            <SingleMetric
              name="Missing"
              value={countQuery.data?.androidCount?.missingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['MISSING'],
              })}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
            />
            <SingleMetric
              name="Dying"
              value={countQuery.data?.androidCount?.dyingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['DYING'],
              })}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <MetricsContainer>
      <Typography variant="h4">Main metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </MetricsContainer>
  );
}

/**
 * MainMetricsContainer is the container component for the main metrics section.
 * It is responsible for fetching the data and passing it to the presentational
 * component.
 */
export function MainMetricsContainer({
  selectedOptions,
  platform,
}: {
  selectedOptions: SelectedOptions;
  platform: Platform;
}) {
  const client = useFleetConsoleClient();

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: stringifyFilters(selectedOptions),
      platform: platform,
    }),
  );

  const labstationsQuery = useQuery({
    ...client.CountDevices.query({
      filter: stringifyFilters({ ...selectedOptions, ...LABSTATION_FILTERS }),
      platform: platform,
    }),
    enabled: platform === Platform.CHROMEOS,
  });

  switch (platform) {
    case Platform.ANDROID:
      return <AndroidMainMetrics countQuery={countQuery} />;
    case Platform.CHROMEOS:
      return (
        <ChromeOSMainMetrics
          countQuery={countQuery}
          labstationsQuery={labstationsQuery}
        />
      );
  }

  return (
    <ChromeOSMainMetrics
      countQuery={countQuery}
      labstationsQuery={labstationsQuery}
    />
  );
}
