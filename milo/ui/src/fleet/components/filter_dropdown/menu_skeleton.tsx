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

import { Box, MenuList, MenuItem, Skeleton } from '@mui/material';

export function MenuSkeleton({
  itemCount,
  maxHeight,
}: {
  itemCount: number;
  maxHeight: number;
}) {
  return (
    <Box key="menu-container-skeleton">
      <MenuList
        sx={{
          overflowY: 'auto',
        }}
        tabIndex={-1}
        key="menu-skeleton"
      >
        <Box
          sx={{
            width: 280,
            maxHeight: maxHeight,
            px: '10px',
          }}
          key="options-container-skeleton"
        >
          {Array.from({ length: itemCount }).map((_, index) => (
            <MenuItem
              sx={{
                height: 30,
                display: 'flex',
                alignItems: 'center',
                padding: '4px 0',
              }}
              key={`option-${index}-skeleton`}
            >
              <Skeleton
                variant="rectangular"
                sx={{
                  width: 20,
                  height: 20,
                  marginRight: 1,
                }}
              />
              <Skeleton
                variant="text"
                height={32}
                sx={{ width: '100%', marginBottom: '1px' }}
              />
            </MenuItem>
          ))}
        </Box>
      </MenuList>
    </Box>
  );
}
