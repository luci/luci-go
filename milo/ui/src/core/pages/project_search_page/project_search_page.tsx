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

import { Alert, Link, Typography } from '@mui/material';
import Box from '@mui/material/Box';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useDeclarePageId } from '@/common/components/page_meta';
import { SearchInput } from '@/common/components/search_input';
import { UiPage } from '@/common/constants/view';
import { getLoginUrl } from '@/common/tools/url_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ProjectList } from './project_list';

export function ProjectSearchPage() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const searchQuery = searchParams.get('q') || '';
  const authState = useAuthState();

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
    <Box sx={{ px: 6, py: 5, maxWidth: '950px', margin: '0 auto' }}>
      <Box sx={{ mx: 20 }}>
        <SearchInput
          placeholder="Filter projects"
          onValueChange={handleSearchQueryChange}
          value={searchQuery}
          initDelayMs={100}
          // This is the sole purpose of the page. It's OK to autofocus.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
        />
      </Box>
      <Box sx={{ m: 5 }}>
        <ProjectList searchQuery={searchQuery} />
      </Box>
      {authState.identity === ANONYMOUS_IDENTITY && (
        <Alert severity="info">
          {"Can't see the project you expected? Try "}
          <Link
            href={getLoginUrl(
              location.pathname + location.search + location.hash,
            )}
          >
            logging in
          </Link>
          .
        </Alert>
      )}
      <Typography component="div">
        <p>
          <strong>Welcome to LUCI,</strong> the{' '}
          <em>Layered Universal Continuous Integration</em> system.
        </p>
        <p>
          LUCI is used for building and testing some of the largest open source
          projects at Google, such as Chrome and ChromeOS.
        </p>
        <p>
          If you are a Googler, you can find docs, on-boarding and more at{' '}
          <Link
            target="_blank"
            href="https://goto.google.com/luci"
            rel="noreferrer"
          >
            go/luci
          </Link>
          .
        </p>
      </Typography>
    </Box>
  );
}

export function Component() {
  useDeclarePageId(UiPage.ProjectSearch);

  return (
    <TrackLeafRoutePageView contentGroup="project-search">
      <title>Projects</title>
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="project-search"
      >
        <ProjectSearchPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
