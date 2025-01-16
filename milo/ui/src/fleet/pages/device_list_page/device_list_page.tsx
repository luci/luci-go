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

import { Divider } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import { useEffect, useState } from 'react';
import { Helmet } from 'react-helmet';

import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { DeviceTable } from '@/fleet/components/device_table';
import { MainMetrics } from '@/fleet/components/main_metrics';
import { MultiSelectFilter } from '@/fleet/components/multi_select_filter';
import {
  filtersUpdater,
  getFilters,
} from '@/fleet/components/multi_select_filter/search_param_utils/search_param_utils';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { Option, SelectedOptions } from '@/fleet/types';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { GetDeviceDimensionsResponse } from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const DeviceListPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const [selectedOptions, setSelectedOptions] = useState<SelectedOptions>(
    getFilters(searchParams),
  );

  useEffect(() => {
    setSearchParams(filtersUpdater(selectedOptions));
  }, [selectedOptions, setSearchParams]);

  const client = useFleetConsoleClient();
  const dimensionsQuery = useQuery(client.GetDeviceDimensions.query({}));

  return (
    <div>
      <MainMetrics />
      <Divider flexItem color={colors.grey[300]} />
      {dimensionsQuery.data && (
        <MultiSelectFilter
          filterOptions={toFilterOptions(dimensionsQuery.data)}
          selectedOptions={selectedOptions}
          setSelectedOptions={setSelectedOptions}
        />
      )}
      <DeviceTable filter={selectedOptions} />
    </div>
  );
};

const toFilterOptions = (response: GetDeviceDimensionsResponse): Option[] => {
  const baseDimensions = Object.entries(response.baseDimensions).map(
    ([key, value]) => {
      return {
        label: key,
        value: key,
        options: value.values.map((value) => {
          return { label: value, value: value };
        }),
      } as Option;
    },
  );

  const labels = Object.entries(response.labels).flatMap(([key, value]) => {
    // We need to avoid duplicate options
    // E.g. `dut_id` is in both base dimensions and labels
    if (response.baseDimensions[key]) {
      return [];
    }

    return [
      {
        label: key,
        value: 'labels.' + key,
        options: value.values.map((value) => {
          return { label: value, value: value };
        }),
      } as Option,
    ];
  });

  return baseDimensions.concat(labels);
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-device-list">
      <Helmet>
        <title>Streamlined Fleet UI</title>
        <link rel="icon" href={bassFavicon} />
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-device-list-page"
      >
        <DeviceListPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
