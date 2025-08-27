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

import { Menu, MenuItem } from '@mui/material';

import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

interface SelectGroupMenuProps {
  anchorEl: HTMLElement | null;
  /**
   * When group is present, the selected items will be moved to the given group.
   * When group is undefined, the menu will be closed with no items being moved.
   */
  onSelect: (group: AlertGroup) => void;
  onClose: () => void;
  groups: readonly AlertGroup[];
}
export const SelectGroupMenu = ({
  anchorEl,
  onSelect,
  onClose,
  groups,
}: SelectGroupMenuProps) => {
  const open = Boolean(anchorEl);

  return (
    <>
      <Menu
        id="basic-menu"
        anchorEl={anchorEl}
        open={open}
        onClose={() => onClose()}
      >
        {groups.map((group) => (
          <MenuItem
            key={group.name}
            onClick={(e) => {
              e.stopPropagation();
              onSelect(group);
              onClose();
            }}
          >
            {group.displayName}
          </MenuItem>
        ))}
      </Menu>
    </>
  );
};
