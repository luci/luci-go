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

import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import { styled } from '@mui/material/styles';

// Base IconButton style
export const StyledIconButton = styled(IconButton)(() => ({
  color: 'var(--gm3-color-on-surface-variant)',
})) as typeof IconButton;

export const StyledVerticalDivider = styled(Divider)(() => ({
  borderColor: 'var(--gm3-color-outline-variant)',
})) as typeof Divider;

export const StyledAppBarText = styled(Typography)(() => ({
  fontWeight: 500,
  fontSize: '22px',
  lineHeight: '28px',
  color: 'var(--gm3-color-on-surface-variant)',
})) as typeof Typography;

export const StyledAppBarLink = styled(Link)(() => ({
  fontWeight: 500,
  fontSize: '22px',
  lineHeight: '28px',
  color: 'var(--gm3-color-on-surface-variant)',
})) as typeof Link;

export const StyledToolbar = styled(Toolbar)(({ theme }) => ({
  minHeight: '64px',
  paddingLeft: theme.spacing(3),
  paddingRight: theme.spacing(3),
})) as typeof Toolbar;
