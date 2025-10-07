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
import './groups.css';

import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import TextField from '@mui/material/TextField';
import { useQuery } from '@tanstack/react-query';
import { useState, forwardRef, useImperativeHandle, useRef } from 'react';
import { Link } from 'react-router';
import { Virtuoso, VirtuosoHandle } from 'react-virtuoso';

import { GroupsListItem } from '@/authdb/components/groups_list_item';
import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';
import { getAuthGroupURLPath } from '@/common/tools/url_utils';
import {
  AuthGroup,
  ListGroupsRequest,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

interface GroupsListProps {
  selectedGroup: string;
}

export interface GroupsListElement {
  refetchList: (fresh: boolean) => void;
  scrollToGroup: (name: string) => void;
}

export const GroupsList = forwardRef<GroupsListElement, GroupsListProps>(
  function GroupList({ selectedGroup }, ref) {
    const [forceFresh, setForceFresh] = useState(false);
    const [filteredGroups, setFilteredGroups] = useState<AuthGroup[]>();
    const virtuosoRef = useRef<VirtuosoHandle>(null);
    const [visibleRange, setVisibleRange] = useState({
      startIndex: 0,
      endIndex: 0,
    });

    const client = useAuthServiceGroupsClient();
    const {
      isPending,
      isError,
      data: response,
      error,
      refetch,
    } = useQuery({
      ...client.ListGroups.query(
        ListGroupsRequest.fromPartial({
          fresh: forceFresh,
        }),
      ),
      refetchOnWindowFocus: false,
    });
    const allGroups: readonly AuthGroup[] = response?.groups || [];

    useImperativeHandle(ref, () => ({
      refetchList: (fresh: boolean) => {
        if (fresh !== forceFresh) {
          setForceFresh(fresh);
          // The change in the state will cause a re-render. No need to call
          // refetch anymore.
          return;
        }

        if (refetch) {
          refetch();
        }
      },
      scrollToGroup: (name: string) => {
        scrollToGroup(name);
      },
    }));

    const changeSearchQuery = (query: string) => {
      setFilteredGroups(
        allGroups.filter((group) => group.name.includes(query.toLowerCase())),
      );
    };

    const groups = filteredGroups ? filteredGroups : allGroups;

    const getIndexToSelect = () => {
      return Math.max(
        groups.findIndex((g) => g.name === selectedGroup),
        0,
      );
    };

    const scrollToGroup = (groupName: string) => {
      let indexToSelect: number = 0;
      for (let i = 0; i < groups.length; i++) {
        if (groups[i].name === groupName) {
          indexToSelect = i;
          break;
        }
      }
      if (
        !(
          indexToSelect >= visibleRange.startIndex &&
          indexToSelect <= visibleRange.endIndex
        )
      ) {
        virtuosoRef.current?.scrollToIndex({
          index: indexToSelect,
        });
      }
    };

    if (isPending) {
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

    return (
      <>
        <Box>
          <Button
            variant="contained"
            disableElevation
            sx={{ m: '16px', mb: 0 }}
            data-testid="create-button"
            component={Link}
            to={getAuthGroupURLPath('new!')}
            endIcon={<AddCircleOutlineIcon />}
          >
            Create Group
          </Button>
        </Box>
        <Box sx={{ p: 2 }}>
          <TextField
            id="outlined-basic"
            label="Search for an existing group"
            variant="outlined"
            style={{ width: '100%' }}
            onChange={(e) => changeSearchQuery(e.target.value)}
          />
        </Box>
        <Box sx={{ height: '100%' }} data-testid="groups-list">
          <Virtuoso
            ref={virtuosoRef}
            style={{ height: '100%' }}
            totalCount={groups.length}
            rangeChanged={setVisibleRange}
            initialTopMostItemIndex={getIndexToSelect()}
            itemContent={(index) => {
              return (
                <GroupsListItem
                  group={groups[index]}
                  selected={groups[index].name === selectedGroup}
                  key={index}
                ></GroupsListItem>
              );
            }}
          />
        </Box>
      </>
    );
  },
);
