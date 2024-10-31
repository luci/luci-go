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

import '../components/groups_list.css';

import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import { useEffect, createRef } from 'react';

import { GroupsForm } from '@/authdb/components/groups_form';
import { GroupsFormNew } from '@/authdb/components/groups_form_new';
import { GroupsList } from '@/authdb/components/groups_list';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useNavigate, useParams } from 'react-router-dom';
import { getURLPathFromAuthGroup } from '@/common/tools/url_utils';
import { GroupsListElement } from '@/authdb/components/groups_list';

export function GroupsPage() {
  const { ['__luci_ui__-raw-*']: groupName } = useParams();
  const navigate = useNavigate();
  const listRef = createRef<GroupsListElement>();

  useEffect(() => {
    if (!groupName) {
      navigate(getURLPathFromAuthGroup('administrators'), { replace: true });
    }
  }, [navigate, groupName]);

  if (!groupName) {
    return <></>;
  }

  const refetchGroups = () => {
    listRef.current?.refetchList();
  }

  return (
    <Paper className="groups-container-paper">
      <Grid container className="groups-container">
        <Grid
          item
          xs={4}
          className="container-left"
          sx={{ display: 'flex', flexDirection: 'column' }}
        >
          <GroupsList selectedGroup={groupName} ref={listRef}/>
        </Grid>
        <Grid
          item
          xs={8}
          className="container-right"
          sx={{ display: 'flex', flexDirection: 'column' }}
        >
          <Box className="groups-details-container">
            {groupName === 'new!' ? (
              <GroupsFormNew onCreate={refetchGroups}/>
            ) : (
              <>
                <GroupsForm
                  key={groupName}
                  name={groupName}
                  onDelete={refetchGroups}
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
  return (
    <TrackLeafRoutePageView contentGroup="authdb-group">
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
