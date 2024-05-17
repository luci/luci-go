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

import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';
import { GroupsListItem } from '@/authdb/components/groups_list_item';
import { useState } from 'react';
import Box from '@mui/material/Box';
import List from '@mui/material/List';

export function GroupsList() {
  const [allGroups, setAllGroups] = useState<readonly AuthGroup[]>();

  const client = useAuthServiceClient();
  client.ListGroups({}).then((response) => {
    setAllGroups(response?.groups);
  });

  return (
    <Box className="groups-list-container">
        <List >
            {allGroups && allGroups.map((group) => (
                <GroupsListItem key={group.name} group={group} />
            ))}
        </List>
    </Box>
  );
}