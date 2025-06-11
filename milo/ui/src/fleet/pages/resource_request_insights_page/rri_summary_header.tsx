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
import { Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

import { useRriFilters } from './use_rri_filters';

export function RriSummaryHeader() {
  const client = useFleetConsoleClient();

  const { filterData, setFilters } = useRriFilters();

  const { data, isLoading, isError } = useQuery(
    client.CountResourceRequests.query({
      filter: '', // TODO: b/396079336 add filtering
    }),
  );

  /**
   * @param filterName name of the filter, ex. state
   * @param filterValue array of filter values, ex. DEVICE_STATE_LEASED
   * @returns will return URL query part with filters and existing parameters, like sorting
   */
  // TODO: b/421989100 - share this code with other main metrics panel code.
  const addFilter = (filterName: string, filterValue: string[]) => () => {
    const newFilters = { ...filterData, [filterName]: filterValue };
    setFilters(newFilters);
  };

  if (isError) {
    return <Typography variant="h4">Error</Typography>; // TODO: b/397421370 improve this
  }

  return (
    <MetricsContainer>
      <Typography variant="h4">All Requests</Typography>
      <div css={{ marginTop: 24 }}>
        <Typography variant="subhead1">Request status</Typography>
        <div
          css={{
            display: 'flex',
            justifyContent: 'space-around',
            marginTop: 5,
          }}
        >
          <SingleMetric
            name="In Progress"
            value={data?.inProgress}
            total={data?.total}
            loading={isLoading}
            handleClick={addFilter('fulfillment_status', ['IN_PROGRESS'])}
          />
          <SingleMetric
            name="Completed"
            value={data?.completed}
            total={data?.total}
            loading={isLoading}
            handleClick={addFilter('fulfillment_status', ['COMPLETED'])}
          />
          <SingleMetric
            name="Material Sourcing"
            value={data?.materialSourcing}
            total={data?.total}
            loading={isLoading}
          />
          <SingleMetric
            name="Build"
            value={data?.build}
            total={data?.total}
            loading={isLoading}
          />
          <SingleMetric
            name="QA"
            value={data?.qa}
            total={data?.total}
            loading={isLoading}
          />
          <SingleMetric
            name="Config"
            value={data?.config}
            total={data?.total}
            loading={isLoading}
          />
        </div>
      </div>
    </MetricsContainer>
  );
}
