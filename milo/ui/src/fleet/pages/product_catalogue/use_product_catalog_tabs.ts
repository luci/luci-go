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

import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useNavigate } from 'react-router';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export enum ProductCatalogTab {
  ALL = 'All',
  ANDROID_TESTBED = 'android-testbed',
  HARDWARE = 'hardware',
  OS_TESTBED = 'os-testbed',
  PERIPHERALS = 'peripherals',
}

export function useProductCatalogTabs() {
  const [searchParams] = useSyncedSearchParams();
  const client = useFleetConsoleClient();
  const urlTab =
    (searchParams.get('tab') as ProductCatalogTab) || ProductCatalogTab.ALL;

  const filterOptionsQuery = useQuery({
    ...client.GetProductCatalogFilterValues.query({ filter: '' }),
    placeholderData: keepPreviousData,
  });

  const tabs = useMemo(() => {
    const apiTabs = filterOptionsQuery.data?.scopedProductType || [];
    if (apiTabs.length > 0) {
      const isAnyInScope = apiTabs.some((t) => t.inScope);
      return [
        { value: ProductCatalogTab.ALL as string, inScope: isAnyInScope },
        ...apiTabs,
      ];
    }
    return [];
  }, [filterOptionsQuery.data?.scopedProductType]);

  const navigate = useNavigate();

  //Ideally we would prefer UseState so UseTabs wouldn't have any logic in View and filters
  const handleTabChange = (_: React.SyntheticEvent, newValue: string) => {
    const currentView = searchParams.get('view');
    navigate(
      `/ui/fleet/catalog?tab=${newValue}&view=${currentView || 'table'}`,
    );
  };

  return {
    tabs,
    selectedTab: Object.values(ProductCatalogTab).includes(urlTab)
      ? urlTab
      : ProductCatalogTab.ALL,
    handleTabChange,
  };
}
