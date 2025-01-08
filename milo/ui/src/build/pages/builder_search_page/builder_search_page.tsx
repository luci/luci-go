// Copyright 2023 The LUCI Authors.
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

import Box from '@mui/material/Box';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta, usePageId } from '@/common/components/page_meta';
import { SearchInput } from '@/common/components/search_input';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { BuilderList } from './builder_list';

export function BuilderSearchPage() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const searchQuery = searchParams.get('q') || '';

  const handleSearchQueryChange = (newSearchQuery: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (newSearchQuery === '') {
        next.delete('q');
      } else {
        next.set('q', newSearchQuery);
      }
      return next;
    });
  };

  return (
    <Box sx={{ px: 6, py: 2 }}>
      <PageMeta title="Builder search" />
      <Box sx={{ mx: 20 }}>
        <SearchInput
          placeholder="Search builders"
          onValueChange={(v) => handleSearchQueryChange(v)}
          value={searchQuery}
          initDelayMs={300}
        />
      </Box>
      <Box sx={{ mt: 5 }}>
        <BuilderList searchQuery={searchQuery} />
      </Box>
    </Box>
  );
}

export function Component() {
  usePageId(UiPage.BuilderSearch);

  return (
    <TrackLeafRoutePageView contentGroup="builder-search">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="builder-search"
      >
        <BuilderSearchPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
