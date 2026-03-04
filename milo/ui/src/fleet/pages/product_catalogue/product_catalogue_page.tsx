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
import { useQuery } from '@tanstack/react-query';
import { MaterialReactTable } from 'material-react-table';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

const Container = styled.div`
  margin: 24px;
`;

export const ProductCatalogListPage = () => {
  const client = useFleetConsoleClient();
  const query = useQuery(client.ListProductCatalogEntries.query({}));

  const table = useFCDataTable({
    columns: [
      {
        id: 'productCatalogId',
        header: 'Product Catalog ID',
        accessorKey: 'productCatalogId',
      },
      {
        id: 'productName',
        header: 'Product Name',
        accessorKey: 'productName',
      },
      {
        id: 'gpn',
        header: 'GPN',
        accessorKey: 'gpn',
      },
      {
        id: 'descriptiveName',
        header: 'Descriptive Name',
        accessorKey: 'descriptiveName',
      },
      {
        id: 'resourceType',
        header: 'Resource Type',
        accessorKey: 'resourceType',
      },
      {
        id: 'fleetPlmStatus',
        header: 'Fleet PLM Status',
        accessorKey: 'fleetPlmStatus',
      },
      {
        id: 'r11n',
        header: 'R11N',
        accessorKey: 'r11n',
      },
      {
        id: 'numberOfDevicesPerRack',
        header: 'Number of Devices Per Rack',
        accessorKey: 'numberOfDevicesPerRack',
      },
      {
        id: 'unitCost',
        header: 'Unit Cost',
        accessorKey: 'unitCost',
      },
    ],
    data: [...(query.data?.entries ?? [])],
    enablePagination: true,
  });

  return (
    <Container>
      <MaterialReactTable table={table} />
    </Container>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-product-catalog-list">
      <FleetHelmet pageTitle="Product Catalog" />
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
          <ProductCatalogListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
