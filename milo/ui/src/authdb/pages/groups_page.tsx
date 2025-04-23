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

import '../components/groups.css';

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid2';
import Paper from '@mui/material/Paper';
import { createRef, useEffect } from 'react';
import { Helmet } from 'react-helmet';
import { useNavigate, useParams } from 'react-router-dom';

import { GroupDetails } from '@/authdb/components/group_details';
import { GroupsFormNew } from '@/authdb/components/groups_form_new';
import { GroupsList, GroupsListElement } from '@/authdb/components/groups_list';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useDeclarePageId } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { getURLPathFromAuthGroup } from '@/common/tools/url_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export function GroupsPage() {
  const { ['__luci_ui__-raw-*']: groupName } = useParams();
  const navigate = useNavigate();
  const listRef = createRef<GroupsListElement>();
  const [searchParams] = useSyncedSearchParams();

  useEffect(() => {
    if (!groupName) {
      navigate(
        getURLPathFromAuthGroup('administrators', searchParams.get('tab')),
        {
          replace: true,
        },
      );
    }
    if (groupName) {
      listRef.current?.scrollToGroup(groupName);
    }
  }, [navigate, groupName, listRef, searchParams]);

  if (!groupName) {
    return <></>;
  }

  const refetchGroups = (fresh: boolean) => {
    listRef.current?.refetchList(fresh);
  };

  return (
    <Paper className="groups-container-paper">
      <Alert severity="info">
        <AlertTitle> You are using the new UI for Auth Service.</AlertTitle>
        If there is additional functionality you would like supported, file a
        bug using the{' '}
        <a
          href="https://b.corp.google.com/issues/new?component=1435307&template=2026255"
          target="_blank"
          rel="noreferrer"
        >
          feedback link
        </a>{' '}
        and use the{' '}
        <a
          href={`https://${SETTINGS.authService.host}/auth/groups`}
          target="_blank"
          rel="noreferrer"
        >
          previous UI
        </a>{' '}
        in the meantime.
      </Alert>
      <Grid container className="groups-container">
        <Grid
          size={{ xs: 4 }}
          className="container-left"
          sx={{
            display: 'flex',
            flexDirection: 'column',
            borderRight: '1px solid #bdbdbd',
          }}
        >
          <GroupsList selectedGroup={groupName} ref={listRef} />
        </Grid>
        <Grid
          size={{ xs: 8 }}
          className="container-right"
          sx={{ display: 'flex', flexDirection: 'column' }}
        >
          <Box className="groups-details-container">
            {groupName === 'new!' ? (
              // The list of groups must be the latest, to ensure the group that
              // was just created is included.
              <GroupsFormNew onCreate={() => refetchGroups(true)} />
            ) : (
              <>
                <GroupDetails
                  key={groupName}
                  name={groupName}
                  refetchList={refetchGroups}
                />
              </>
            )}
          </Box>
        </Grid>
      </Grid>
    </Paper>
  );
}

export function Component() {
  useDeclarePageId(UiPage.AuthServiceGroups);

  return (
    <TrackLeafRoutePageView contentGroup="authdb-group">
      <Helmet>
        <title>Groups</title>
      </Helmet>
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="authdb-group"
      >
        <GroupsPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
