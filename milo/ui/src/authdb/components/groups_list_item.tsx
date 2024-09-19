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
import LockIcon from '@mui/icons-material/Lock';
import IconButton from '@mui/material/IconButton';

interface GroupsItemProps {
    readonly group: AuthGroup;
    setSelected: () => void;
    selected: boolean;
}

// True if group name starts with '<something>/' prefix, where
// <something> is a non-empty string.
function isExternalGroupName(name: string) {
    return name.indexOf('/') > 0;
}

export function GroupsListItem({ group, setSelected, selected } :GroupsItemProps) {
  const isExternal = isExternalGroupName(group.name);
  const description = isExternal ? 'External' : group.description;

  return (
      <ListItem disablePadding sx={{maxWidth:'95vw'}} style={{backgroundColor: group.callerCanModify ? 'white': '#ECECEC'}}>
      <ListItemButton
       onClick={setSelected}
       selected={selected}
        >
        <ListItemText
            primary={group.name}
            secondary={description}
            data-testid="groups_item_list_item_text"
            secondaryTypographyProps={{
              style: {
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis'
            }
        }}
        />
      {!group.callerCanModify &&
      <IconButton>
          <LockIcon />
      </IconButton>
      }
      </ListItemButton>
    </ListItem>
  );
}
