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
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import List from '@mui/material/List';
import TextField from '@mui/material/TextField';
import { useQuery } from '@tanstack/react-query';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';
import { GroupsListItem } from '@/authdb/components/groups_list_item';
import {useState, forwardRef, useImperativeHandle} from 'react';

interface GroupsListProps {
  selectionChanged: (name: string) => void;
  createFormSelected: (selected: boolean) => void;
}

export interface GroupsListElement {
  selectFirstGroup: () => void;
}

export const GroupsList = forwardRef<GroupsListElement, GroupsListProps>(
  (
  {selectionChanged, createFormSelected }, ref
  ) => {
  const [filteredGroups, setFilteredGroups] = useState<AuthGroup[]>();
  const [selectedGroup, setSelectedGroup] = useState<string>("");

  useImperativeHandle(ref, () => ({
    selectFirstGroup: () => {
      if (allGroups[0]) {
        handleSelection(allGroups[0].name);
      }
    },
  }));

  const handleSelection = (name: string) => {
    if (selectedGroup == name) {
      return;
    }
    createFormSelected(false);
    selectionChanged(name);
    setSelectedGroup(name);
  };

  const client = useAuthServiceClient();
  const {
    isLoading,
    isError,
    data: response,
    error,
  } = useQuery({
    ...client.ListGroups.query({}),
    onSuccess: (response) => {
      if (selectedGroup == "") {
        handleSelection(response?.groups[0].name);
      }
    },
    refetchOnWindowFocus: false,
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

  const changeSearchQuery = (query: string) => {
    setFilteredGroups(allGroups.filter(group => group.name.includes(query.toLowerCase())));
  }

  const groups = (filteredGroups) ? filteredGroups : allGroups;
  return (
    <>
    <Box sx={{ p: 2 }}>
      <TextField
        id="outlined-basic"
        label="Search for an existing group"
        variant="outlined"
        style={{ width: '100%' }}
        onChange={e => changeSearchQuery(e.target.value)} />
    </Box>
    <Box>
      <Button variant="contained" disableElevation sx={{ m: '16px', mt: 0 }} data-testid='create-button' onClick={() => createFormSelected(true)}>
        Create Group
      </Button>
    </Box>
    <Box className="groups-list-container">
      <List data-testid="groups-list" disablePadding>
        {groups.map((group) => (
          <GroupsListItem
            key={group.name}
            group={group}
            setSelected={() => { handleSelection(group.name) }}
            selected={group.name === selectedGroup}
          />
        ))}
      </List>
    </Box>
    </>
  );
}
);