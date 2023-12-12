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
import { ChangeEvent } from 'react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { getLoginUrl } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { SearchInput } from '../search_input';

import { ProjectList } from './project_list';

export const ProjectSearch = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const searchQuery = searchParams.get('q') || '';
  const authState = useAuthState();

  const handleSearchQueryChange = (
    e: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>,
  ) => {
    const pendingSearchQuery = e.target.value;
    if (pendingSearchQuery === '') {
      searchParams.delete('q');
    } else {
      searchParams.set('q', pendingSearchQuery);
    }
    setSearchParams(searchParams);
  };

  return (
    <Box sx={{ px: 6, py: 5, maxWidth: '950px', margin: '0 auto' }}>
      <PageMeta title="Projects" selectedPage={UiPage.ProjectSearch} />
      <SearchInput
        placeholder="Filter projects"
        onInputChange={handleSearchQueryChange}
        value={searchQuery}
        // This is the sole purpose of the page. It's OK to autofocus.
        // eslint-disable-next-line jsx-a11y/no-autofocus
        autoFocus
      />
      <Box sx={{ m: 5 }}>
        <ProjectList searchQuery={searchQuery} />
      </Box>
      {authState.identity == ANONYMOUS_IDENTITY && (
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
};

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="project-search">
    <ProjectSearch />
  </RecoverableErrorBoundary>
);
