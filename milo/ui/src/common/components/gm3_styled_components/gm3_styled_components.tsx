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

// Import base MUI components and props types
import { Divider, IconButton, Link, Toolbar, Typography } from '@mui/material';
import Paper, { PaperProps } from '@mui/material/Paper';
import { styled } from '@mui/material/styles';

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

interface StyledActionBlockProps extends PaperProps {
  severity?: 'primary' | 'warning' | 'error' | 'success' | 'info';
}
export const StyledActionBlock = styled(Paper, {
  shouldForwardProp: (prop) => prop !== 'severity',
})<StyledActionBlockProps>(({ theme, severity = 'primary' }) => ({
  padding: '16px',
  borderRadius: '8px',
  display: 'flex',
  flexDirection: 'row',
  alignItems: 'flex-start',
  gap: '10px',
  boxShadow: 'none',
  ...(severity === 'primary' && {
    background: theme.palette.gm3.primaryContainer,
    '& .MuiSvgIcon-root': { color: theme.palette.primary.main },
    '& .MuiTypography-root, & .MuiLink-root': {
      color: theme.palette.primary.main,
      fontWeight: 700,
      fontSize: '16px',
      lineHeight: '24px',
      letterSpacing: '0.1px',
    },
  }),
  ...(severity === 'warning' && {
    background: theme.palette.gm3.warningContainer,
    '& .MuiSvgIcon-root': { color: theme.palette.gm3.warningDark },
    '& .MuiTypography-root, & .MuiLink-root': {
      color: theme.palette.warning.main,
      fontWeight: 700,
      fontSize: '16px',
      lineHeight: '24px',
      letterSpacing: '0.1px',
    },
  }),
  ...(severity === 'error' && {
    background: theme.palette.gm3.errorContainer,
    '& .MuiSvgIcon-root': { color: theme.palette.error.main },
    '& .MuiTypography-root, & .MuiLink-root': {
      color: theme.palette.error.main,
      fontWeight: 700,
      fontSize: '16px',
      lineHeight: '24px',
      letterSpacing: '0.1px',
    },
  }),
  // Add 'success' and 'info' if needed
  '& .MuiSvgIcon-root': { width: '24px', height: '24px', marginTop: '0px' },
  '& .MuiTypography-root, & .MuiLink-root': { flexGrow: 1 },
}));
