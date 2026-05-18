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

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { LeaseStateInfo } from '@/fleet/components/lease_state_info/lease_state_info';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getFilterKey } from './chromeos_fields';
import { useChromeOSFilters } from './use_chromeos_filters';

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

const LABSTATION_FILTER_STR =
  'labels."label-pool" = ("labstation_tryjob" OR "labstation_main" OR "labstation_canary")';

export function ChromeOSSummaryHeader() {
  const client = useFleetConsoleClient();
  const { filterValues, aip160 } = useChromeOSFilters(() => {});

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: aip160(),
      platform: Platform.CHROMEOS,
    }),
  );

  const labstationsQuery = useQuery({
    ...client.CountDevices.query({
      filter: aip160()
        ? `${aip160()} AND ${LABSTATION_FILTER_STR}`
        : LABSTATION_FILTER_STR,
      platform: Platform.CHROMEOS,
    }),
  });

  const setFilterOptions = (key: string, options: string[]) => {
    const filterKey = getFilterKey(key);
    const filter = filterValues?.[filterKey];
    if (filter instanceof StringListFilterCategory) {
      filter.setSelectedOptions(options);
    }
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
              handleClick={() =>
                setFilterOptions('state', ['DEVICE_STATE_LEASED'])
              }
            />
            <SingleMetric
              name="Available"
              value={countQuery.data?.taskState?.idle}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              handleClick={() =>
                setFilterOptions('state', ['DEVICE_STATE_AVAILABLE'])
              }
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
              handleClick={() => setFilterOptions('dut_state', ['ready'])}
            />
            <SingleMetric
              name="Need repair"
              value={countQuery.data?.deviceState?.needRepair}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              handleClick={() =>
                setFilterOptions('dut_state', ['needs_repair'])
              }
            />
            <SingleMetric
              name="Repair failed"
              value={countQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              handleClick={() =>
                setFilterOptions('dut_state', ['repair_failed'])
              }
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
              handleClick={() =>
                setFilterOptions('dut_state', ['needs_deploy'])
              }
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
              handleClick={() =>
                setFilterOptions('dut_state', ['needs_replacement'])
              }
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              handleClick={() =>
                setFilterOptions('dut_state', ['needs_manual_repair'])
              }
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
              handleClick={() => {
                setFilterOptions('dut_state', ['ready']);
                setFilterOptions('label-pool', [
                  'labstation_tryjob',
                  'labstation_main',
                  'labstation_canary',
                ]);
              }}
            />
            <SingleMetric
              name="Repair failed"
              value={labstationsQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              loading={labstationsQuery.isPending}
              handleClick={() => {
                setFilterOptions('dut_state', ['repair_failed']);
                setFilterOptions('label-pool', [
                  'labstation_tryjob',
                  'labstation_main',
                  'labstation_canary',
                ]);
              }}
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
              handleClick={() => {
                setFilterOptions('dut_state', ['needs_deploy']);
                setFilterOptions('label-pool', [
                  'labstation_tryjob',
                  'labstation_main',
                  'labstation_canary',
                ]);
              }}
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
