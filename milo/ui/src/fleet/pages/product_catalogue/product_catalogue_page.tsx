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

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

const Container = styled.div`
  margin: 24px;
`;

export const ProductCatalogListPage = () => {
  return <Container>{}</Container>;
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
