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
import './groups_list.css';

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import List from '@mui/material/List';
import Paper from '@mui/material/Paper';
import TextField from '@mui/material/TextField';
import { useQuery } from '@tanstack/react-query';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';
import { GroupsListItem } from '@/authdb/components/groups_list_item';
import { useState } from 'react';
import {GroupsForm} from '@/authdb/components/groups_form';
import Grid from '@mui/material/Grid';

export function GroupsList() {
  const [selectedGroup, setSelectedGroup] = useState<string>();
  const [filteredGroups, setFilteredGroups] = useState<AuthGroup[]>();
  const [searchQuery, setSearchQuery] = useState<string>();

  const client = useAuthServiceClient();
  const {
    isLoading,
    isError,
    data: response,
    error,
  } = useQuery({
    ...client.ListGroups.query({})
  })
  const allGroups: readonly AuthGroup[] = response?.groups || [];
  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="groups-list-error">
        <Alert severity="error">
          <AlertTitle>Failed to load groups list</AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  const setSelected = (name: string) => {
    setSelectedGroup(name);
  };

  const changeSearchQuery = (query: string) => {
    setSearchQuery(query.toLowerCase());
    if (searchQuery) {
      setFilteredGroups(allGroups.filter(group => group.name.includes(searchQuery)));
    }
  }

  const groups = searchQuery ? filteredGroups : allGroups;
  return (
    <Paper>
      <Grid container className="groups-container">
        <Grid item xs={5}>
        <TextField
            id="outlined-basic"
            label="Search for an existing group"
            variant="outlined"
            sx={{m: 2}}
            onChange={e => changeSearchQuery(e.target.value)}/>
          <Box className="groups-list-container">
            <List data-testid="groups-list" disablePadding>
              {groups && groups.map((group) => (
              <GroupsListItem
                key={group.name}
                group={group}
                setSelected={() => {setSelected(group.name)}}
                selected={group.name === selectedGroup}
              />
              ))}
            </List>
          </Box>
        </Grid>
        <Grid item xs={7}>
          {selectedGroup && <GroupsForm name={selectedGroup}/>}
        </Grid>
      </Grid>
    </Paper>
  );
}