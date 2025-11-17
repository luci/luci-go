// Copyright 2025 The LUCI Authors.
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

import { type CSSObject } from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { LeaseStateInfo } from '@/fleet/components/lease_state_info/lease_state_info';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

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

export interface ChromeOSSummaryHeaderProps {
  selectedOptions: SelectedOptions;
}

/**
 * MainMetrics is the presentational component for the main metrics section.
 * It is responsible for displaying the metrics, but not for fetching the data.
 */
export function ChromeOSSummaryHeader({
  selectedOptions,
}: ChromeOSSummaryHeaderProps) {
  const client = useFleetConsoleClient();

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: stringifyFilters(selectedOptions),
      platform: Platform.CHROMEOS,
    }),
  );

  const labstationsQuery = useQuery({
    ...client.CountDevices.query({
      filter: stringifyFilters({ ...selectedOptions, ...LABSTATION_FILTERS }),
      platform: Platform.CHROMEOS,
    }),
  });
  const [searchParams, _] = useSyncedSearchParams();

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
              filterUrl={getFilterQueryString(
                {
                  state: ['DEVICE_STATE_LEASED'],
                },
                searchParams,
              )}
            />
            <SingleMetric
              name="Available"
              value={countQuery.data?.taskState?.idle}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                {
                  state: ['DEVICE_STATE_AVAILABLE'],
                },
                searchParams,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['ready'], // TODO: Hotfix for b/449956551, needs further investigation on quote handling
                },
                searchParams,
              )}
            />
            <SingleMetric
              name="Need repair"
              value={countQuery.data?.deviceState?.needRepair}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['needs_repair'],
                },
                searchParams,
              )}
            />
            <SingleMetric
              name="Repair failed"
              value={countQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['repair_failed'],
                },
                searchParams,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['needs_deploy'],
                },
                searchParams,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['needs_replacement'],
                },
                searchParams,
              )}
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['needs_manual_repair'],
                },
                searchParams,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['ready'],
                  ...LABSTATION_FILTERS,
                },
                searchParams,
              )}
            />
            <SingleMetric
              name="Repair failed"
              value={labstationsQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              loading={labstationsQuery.isPending}
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['repair_failed'],
                  ...LABSTATION_FILTERS,
                },
                searchParams,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  'labels."dut_state"': ['needs_deploy'],
                  ...LABSTATION_FILTERS,
                },
                searchParams,
              )}
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
