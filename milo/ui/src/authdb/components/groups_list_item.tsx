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
import LockIcon from '@mui/icons-material/Lock';
import IconButton from '@mui/material/IconButton';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import Tooltip from '@mui/material/Tooltip';
import { Link as RouterLink } from 'react-router-dom';

import { getURLPathFromAuthGroup } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

interface GroupsItemProps {
  readonly group: AuthGroup;
  selected: boolean;
}

// True if group name starts with '<something>/' prefix, where
// <something> is a non-empty string.
function isExternalGroupName(name: string) {
  return name.indexOf('/') > 0;
}
export function GroupsListItem({ group, selected }: GroupsItemProps) {
  const isExternal = isExternalGroupName(group.name);
  const description = isExternal ? 'External' : group.description;
  const [searchParams] = useSyncedSearchParams();

  return (
    <ListItem
      disablePadding
      sx={{ maxWidth: '95vw' }}
      style={{ backgroundColor: group.callerCanModify ? 'white' : '#ECECEC' }}
    >
      <ListItemButton
        selected={selected}
        component={RouterLink}
        to={getURLPathFromAuthGroup(group.name, searchParams.get('tab'))}
      >
        <ListItemText
          primary={group.name}
          secondary={description}
          data-testid="groups_item_list_item_text"
          secondaryTypographyProps={{
            style: {
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            },
          }}
        />
        {!group.callerCanModify && (
          <Tooltip title="View Only">
            <IconButton disableRipple disableFocusRipple>
              <LockIcon />
            </IconButton>
          </Tooltip>
        )}
      </ListItemButton>
    </ListItem>
  );
}
