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
import ListItem from '@mui/material/ListItem';
import ListItemText from '@mui/material/ListItemText';
import ListItemButton from '@mui/material/ListItemButton';
import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

interface GroupsItemProps {
    readonly group: AuthGroup;
}

// True if group name starts with '<something>/' prefix, where
// <something> is a non-empty string.
function isExternalGroupName(name: string) {
    return name.indexOf('/') > 0;
}

// Trims group description to fit single line.
const trimGroupDescription = (desc: string) => {
  if (desc == null) {
    return '';
  }
  let firstLine = desc.split('\n')[0];
  if (firstLine.length > 55) {
    firstLine = firstLine.slice(0, 55) + '...';
  }
  return firstLine;
}

export function GroupsListItem({ group } :GroupsItemProps) {
  let description = isExternalGroupName(group.name) ? 'External' : trimGroupDescription(group.description);

  return (
    <ListItem disablePadding>
    <ListItemButton>
        <ListItemText
            primary={group.name}
            secondary={description}
            data-testid="groups_item_list_item_text" />
    </ListItemButton>
  </ListItem>
);
}
