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

import styled from '@emotion/styled';
import { useEffect } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { useWarnings, WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { ResourceRequestTable } from './resource_requests_table';
import { RriSummaryHeader } from './rri_summary_header';
import { useRriFilters } from './use_rri_filters';
import { useRriUrlMigration } from './use_rri_url_migration';

const Container = styled.div`
  margin: 24px;
`;

export const ResourceRequestListPage = () => {
  const { filterValues, isLoading, parseError } = useRriFilters();
  const [warnings, addWarning] = useWarnings();

  useEffect(() => {
    if (parseError) {
      addWarning(`There was an error parsing your filters: ${parseError}`);
    }
  }, [parseError, addWarning]);

  return (
    <Container>
      <WarningNotifications warnings={warnings} />
      <RriSummaryHeader />
      <div
        css={{
          marginTop: 24,
          width: '100%',
          display: 'flex',
          justifyContent: 'flex-start',
          alignItems: 'center',
          gap: 8,
          borderRadius: 4,
        }}
      >
        <FilterBar
          filterCategoryDatas={Object.values(filterValues || {})}
          isLoading={isLoading}
          searchPlaceholder='Add a filter (e.g. "rr_id" or "status")'
          onApply={() => {}}
        />
      </div>
      <ResourceRequestTable />
    </Container>
  );
};

export function Component() {
  const { isMigrating } = useRriUrlMigration();

  if (isMigrating) {
    return null;
  }

  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-resource-request-list">
      <FleetHelmet pageTitle="Resource Requests" />
      <RecoverableErrorBoundary
        fallbackRender={({ error }) => (
          <LoggedInBoundary>
            <div css={{ padding: 24 }}>
              <>{error instanceof Error ? error.message : String(error)}</>
            </div>
          </LoggedInBoundary>
        )}
      >
        <LoggedInBoundary>
          <ResourceRequestListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
