// Copyright 2026 The LUCI Authors.
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

import { Avatar, AvatarProps } from '@mui/material';
import { forwardRef } from 'react';

import { colors } from '@/fleet/theme/colors';

import { normalizeEmail, useUserPhoto } from './use_user_photo';

// eslint-disable-next-line react-refresh/only-export-components
export const AVATAR_COLORS = [
  colors.blue[700],
  colors.green[700],
  colors.purple[700],
  colors.orange[700],
  colors.red[700],
  colors.cyan[800],
  colors.pink[700],
  colors.yellow[900],
  colors.blue[900],
  colors.orange[900],
];

// eslint-disable-next-line react-refresh/only-export-components
export const getAvatarColor = (identifier: string): string => {
  const normalized = normalizeEmail(identifier).split('@')[0];
  let hash = 0;
  for (let i = 0; i < normalized.length; i++) {
    hash = normalized.charCodeAt(i) + ((hash << 5) - hash);
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
};

// eslint-disable-next-line react-refresh/only-export-components
export const getInitial = (name?: string, email?: string): string => {
  const text = (name?.trim() || email?.trim() || '').replace(/^user:/i, '');
  return text.charAt(0).toUpperCase() || '?';
};

export interface UserAvatarProps extends AvatarProps {
  name?: string;
  email?: string;
  id?: string;
}

export const UserAvatar = forwardRef<HTMLDivElement, UserAvatarProps>(
  ({ name, email, id, sx, children, ...rest }, ref) => {
    const identifier = name || email || id || '';
    const bgColor = getAvatarColor(identifier);
    const initial = children ?? getInitial(name, email || id);
    const photoProps = useUserPhoto({ name, email, id });

    return (
      <Avatar
        ref={ref}
        sx={[
          {
            bgcolor: bgColor,
            fontWeight: 700,
          },
          ...(Array.isArray(sx) ? sx : [sx]),
        ]}
        {...photoProps}
        {...rest}
      >
        {initial}
      </Avatar>
    );
  },
);

UserAvatar.displayName = 'UserAvatar';
