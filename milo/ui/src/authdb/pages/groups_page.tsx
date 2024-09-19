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

import { GroupsList } from '@/authdb/components/groups_list';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';

import '../components/groups_list.css';
import { useState, createRef } from 'react';
import { GroupsForm } from '@/authdb/components/groups_form';
import { GroupsFormNew } from '@/authdb/components/groups_form_new';
import Grid from '@mui/material/Grid';
import Paper from '@mui/material/Paper';
import Box from '@mui/material/Box';
import { GroupsListElement } from '@/authdb/components/groups_list';

export function GroupsPage() {
  const [selectedGroup, setSelectedGroup] = useState<string>("");
  const [showCreateForm, setShowCreateForm] = useState<boolean>();
  const listRef = createRef<GroupsListElement>();

  const selectionChanged = (name: string) => {
    setSelectedGroup(name)
  }

  const onDeletedGroup = () => {
    listRef.current?.selectFirstGroup()
  }

  return (
    <Paper className='groups-container-paper'>
      <Grid container className="groups-container">
        <Grid item xs={4} className="container-left" sx={{ display: 'flex', flexDirection: 'column' }}>
          <GroupsList ref={listRef} selectionChanged={selectionChanged} createFormSelected={(selected) => setShowCreateForm(selected)}/>
        </Grid>
        <Grid item xs={8} className="container-right" sx={{ display: 'flex', flexDirection: 'column' }}>
          <Box className='groups-details-container'>
          {showCreateForm
            ? <GroupsFormNew />
            : <>
              {selectedGroup && <GroupsForm key={selectedGroup} name={selectedGroup} onDelete={onDeletedGroup}/>}
            </>
          }
          </Box>
        </Grid>
      </Grid>
    </Paper>
  );
}

export function Component() {
  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="group">
      <GroupsPage />
    </RecoverableErrorBoundary>
  );
}
