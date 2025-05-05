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

import { createTheme, alpha } from '@mui/material/styles';

const gm3PaletteColors = {
  surface: '#FFFFFF',
  outlineVariant: '#DADCE0',
  onSurfaceVariant: '#5F6368',
  onSurfaceMedium: '#3C4043',
  onSurfaceStrong: '#202124',
  onSurface: '#444746',

  primary: '#1A73E8',
  primaryContainer: '#E8F0FE',
  primaryHoverBg: alpha('#1A73E8', 0.08),

  error: '#D93025',
  errorContainer: '#FCE8E6',

  success: '#1E8E3E',
  successContainer: '#E6F4EA',

  warning: '#F29900',
  warningDark: '#E37400',
  warningContainer: '#FEF7E0',

  surfaceContainer: '#F1F3F4',
  surfaceContainerLow: '#F8F9FA',
};

// Augment the MUI Theme and Palette types for custom keys
declare module '@mui/material/styles' {
  interface Palette {
    gm3: typeof gm3PaletteColors;
  }
  interface PaletteOptions {
    gm3?: Partial<typeof gm3PaletteColors>;
  }
}
declare module '@mui/material/Chip' {
  interface ChipPropsColorOverrides {
    warning: true;
  }
}

export const gm3PageTheme = createTheme({
  palette: {
    mode: 'light',
    primary: { main: gm3PaletteColors.primary },
    error: { main: gm3PaletteColors.error },
    warning: { main: gm3PaletteColors.warning },
    success: { main: gm3PaletteColors.success },
    text: {
      primary: gm3PaletteColors.onSurfaceStrong,
      secondary: gm3PaletteColors.onSurfaceVariant,
    },
    divider: gm3PaletteColors.outlineVariant,
    background: {
      paper: gm3PaletteColors.surface,
    },
    gm3: gm3PaletteColors,
  },
  typography: {
    fontFamily: "'Roboto', sans-serif",
    h5: {
      color: gm3PaletteColors.onSurfaceStrong,
    },
    h6: {
      fontWeight: 400,
      fontSize: '20px',
      color: gm3PaletteColors.onSurfaceStrong,
    },
  },
  components: {
    MuiCard: {
      defaultProps: {
        elevation: 0,
      },
      styleOverrides: {
        root: {
          padding: '24px',
          background: gm3PaletteColors.surface, // theme.palette.background.paper
          border: `1px solid ${gm3PaletteColors.outlineVariant}`, // theme.palette.divider
          borderRadius: '8px',
        },
      },
    },
    MuiTabs: {
      styleOverrides: {
        root: {
          minHeight: '32px',
          borderBottom: `1px solid ${gm3PaletteColors.outlineVariant}`, // theme.palette.divider
        },
        indicator: {
          height: '3px',
          backgroundColor: gm3PaletteColors.primary, // theme.palette.primary.main
        },
      },
    },
    MuiTab: {
      styleOverrides: {
        root: {
          minHeight: '32px',
          padding: '0px 12px',
          textTransform: 'none',
          fontFamily: "'Roboto', sans-serif",
          fontSize: '16px',
          fontWeight: 500,
          lineHeight: '20px',
          letterSpacing: '0.25px',
          color: gm3PaletteColors.onSurfaceVariant, // theme.palette.text.secondary
          '&.Mui-selected': {
            color: gm3PaletteColors.primary, // theme.palette.primary.main
          },
          '&.Mui-focusVisible': {
            backgroundColor: alpha(gm3PaletteColors.primary, 0.1),
          },
        },
      },
    },
    MuiAccordion: {
      defaultProps: {
        disableGutters: true,
        elevation: 0,
      },
      styleOverrides: {
        root: {
          background: gm3PaletteColors.surface, // theme.palette.background.paper
          border: `1px solid ${gm3PaletteColors.outlineVariant}`, // theme.palette.divider
          borderRadius: '8px',
          '&::before': { display: 'none' },
          '&.Mui-expanded': { margin: 0 },
          '& .MuiCollapse-root': {
            borderBottomLeftRadius: '8px',
            borderBottomRightRadius: '8px',
          },
        },
      },
    },
    MuiAccordionSummary: {
      styleOverrides: {
        root: {
          minHeight: '28px',
          padding: '16px',
          color: gm3PaletteColors.onSurfaceStrong, // theme.palette.text.primary
          borderTopLeftRadius: '8px',
          borderTopRightRadius: '8px',
          '&.Mui-expanded': {
            minHeight: '28px',
            borderBottomLeftRadius: 0,
            borderBottomRightRadius: 0,
          },
        },
        expandIconWrapper: {
          color: gm3PaletteColors.onSurfaceVariant, // theme.palette.text.secondary
        },
        content: {
          margin: '0px',
        },
      },
    },
    MuiAccordionDetails: {
      styleOverrides: {
        root: {
          padding: '16px',
        },
      },
    },
    MuiChip: {
      defaultProps: {
        size: 'small',
        variant: 'filled',
      },
      styleOverrides: {
        root: ({ ownerState, theme }) => ({
          borderRadius: '4px',
          height: '20px',
          ...(ownerState.color === 'default' && {
            backgroundColor: theme.palette.gm3.surfaceContainer,
          }),
          ...(ownerState.color === 'primary' && {
            backgroundColor: theme.palette.gm3.primaryContainer,
          }),
          ...(ownerState.color === 'error' && {
            backgroundColor: theme.palette.gm3.errorContainer,
          }),
          ...(ownerState.color === 'success' && {
            backgroundColor: theme.palette.gm3.successContainer,
          }),
          ...(ownerState.color === 'warning' && {
            backgroundColor: theme.palette.gm3.warningContainer,
          }),
        }),
        labelSmall: {
          fontFamily: "'Roboto', sans-serif",
          fontWeight: 400,
          fontSize: '12px',
          lineHeight: '16px',
          letterSpacing: '0.3px',
          color: gm3PaletteColors.onSurfaceMedium,
          paddingLeft: '8px',
          paddingRight: '8px',
        },
        iconSmall: ({ ownerState, theme }) => ({
          fontSize: '16px',
          marginLeft: '4px',
          marginRight: '-4px',
          color:
            ownerState.color === 'primary'
              ? theme.palette.primary.main
              : ownerState.color === 'error'
                ? theme.palette.error.main
                : ownerState.color === 'success'
                  ? theme.palette.success.main
                  : ownerState.color === 'warning'
                    ? theme.palette.gm3.warningDark
                    : theme.palette.gm3.onSurfaceVariant,
        }),
      },
    },
    MuiButton: {
      variants: [
        {
          props: { variant: 'outlined' },
          style: ({ theme }) => ({
            padding: '3px 11px',
            height: '24px',
            background: theme.palette.gm3.surface,
            border: `1px solid ${theme.palette.divider}`,
            borderRadius: '4px',
            textTransform: 'none',
            color: theme.palette.primary.main,
            fontFamily: "'Roboto', sans-serif",
            fontWeight: 500,
            fontSize: '14px',
            lineHeight: '16px',
            letterSpacing: '0.25px',
            gap: '4px',
            '&:hover': {
              background: theme.palette.gm3.primaryHoverBg,
              borderColor: theme.palette.divider,
            },
            '& .MuiButton-startIcon .MuiSvgIcon-root, & .MuiButton-endIcon .MuiSvgIcon-root':
              {
                fontSize: '16px',
                color: theme.palette.primary.main,
              },
          }),
        },
      ],
    },
  },
});
