// Copyright 2025 The LUCI Authors.
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

import { List, ListItemButton, ListItemText, Popover } from '@mui/material';

import { useFormattedCLs } from './context';

interface Props {
  id?: string;
  anchorEl: HTMLElement | null;
  open: boolean;
  onClose: () => void;
}

export function CLsPopover({ id, anchorEl, open, onClose }: Props) {
  const allFormattedCLs = useFormattedCLs();

  return (
    <Popover
      id={id}
      open={open}
      anchorEl={anchorEl}
      onClose={onClose}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      transformOrigin={{ vertical: 'top', horizontal: 'left' }}
    >
      <List dense sx={{ minWidth: 250, maxWidth: 500, p: 1 }}>
        {allFormattedCLs.map((cl) => (
          <ListItemButton
            key={cl.key}
            component="a"
            href={cl.url}
            target="_blank"
            rel="noopener noreferrer"
            sx={{ pl: 1, pr: 1, py: 0.5 }}
          >
            <ListItemText
              primary={cl.display}
              primaryTypographyProps={{
                variant: 'body2',
                style: {
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                },
              }}
            />
          </ListItemButton>
        ))}
      </List>
    </Popover>
  );
}
