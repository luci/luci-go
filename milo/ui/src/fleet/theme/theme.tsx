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

import ClearIcon from '@mui/icons-material/Clear';
import { createTheme, Theme } from '@mui/material/styles';

import { theme as baseTheme } from '@/common/themes/base';
import { DeepPartial } from '@/proto/google/protobuf/empty.pb';

import { colors } from './colors';
import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
} from './tokens';

/**
 * The Fleet Console MUI theme.
 *
 * Component-wide look and feel lives here, not in component `sx` props. If two
 * components need the same style, add a variant or override below (or a recipe
 * in `./recipes.ts`) instead of copying an `sx` block. See src/fleet/GEMINI.md.
 */
export const theme = createTheme(baseTheme, {
  palette: {
    text: {
      primary: fleetColors.textPrimary,
      secondary: fleetColors.textSecondary,
    },
    // background.default stays white: Material React Table uses it for every
    // row. FleetLayout paints the page canvas (fleetColors.canvas) itself.
    background: {
      paper: fleetColors.surface,
    },
    divider: fleetColors.border,
    primary: {
      main: fleetColors.primary,
      dark: fleetColors.primaryHover,
      light: colors.blue[400],
    },
    secondary: {
      main: colors.purple[600],
      dark: colors.purple[800],
      light: colors.purple[400],
    },
    error: {
      main: colors.red[600],
      dark: colors.red[800],
      light: colors.red[400],
    },
    warning: {
      main: colors.yellow[600],
      dark: colors.yellow[800],
      light: colors.yellow[400],
    },
    success: {
      main: colors.green[600],
      dark: colors.green[800],
      light: colors.green[400],
    },
  },
  typography: {
    fontFamily: fontFamilies.text,
    h1: {
      fontFamily: fontFamilies.display,
      fontSize: 36,
      lineHeight: '44px',
      fontWeight: 500,
      letterSpacing: '-0.5px',
    },
    h2: {
      fontFamily: fontFamilies.display,
      fontSize: 32,
      lineHeight: '40px',
      fontWeight: 500,
      letterSpacing: '-0.4px',
    },
    h3: {
      fontFamily: fontFamilies.display,
      fontSize: fontSizes.display,
      lineHeight: '36px',
      fontWeight: 500,
      letterSpacing: '-0.5px',
      fontVariantNumeric: 'tabular-nums',
    },
    h4: {
      fontFamily: fontFamilies.display,
      fontSize: 24,
      lineHeight: '32px',
      fontWeight: 500,
      letterSpacing: '-0.3px',
    },
    h5: {
      fontFamily: fontFamilies.display,
      fontSize: fontSizes.headline,
      lineHeight: '28px',
      fontWeight: 500,
      letterSpacing: '-0.2px',
    },
    h6: {
      fontFamily: fontFamilies.display,
      fontSize: fontSizes.title,
      lineHeight: '24px',
      fontWeight: 500,
      letterSpacing: '-0.1px',
    },
    subhead1: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.title,
      lineHeight: '24px',
      fontWeight: 500,
    },
    subhead2: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.body,
      lineHeight: '20px',
      fontWeight: 500,
    },
    subtitle1: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.title,
      lineHeight: '24px',
      fontWeight: 500,
    },
    subtitle2: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.body,
      lineHeight: '20px',
      fontWeight: 500,
    },
    body1: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.body,
      lineHeight: '22px',
    },
    body2: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.label,
      lineHeight: '20px',
    },
    caption: {
      fontFamily: fontFamilies.text,
      fontSize: fontSizes.caption,
      lineHeight: '16px',
    },
    button: {
      fontFamily: fontFamilies.display,
      fontSize: fontSizes.label,
      fontWeight: 500,
      textTransform: 'none',
    },
  },
  components: {
    MuiButton: {
      styleOverrides: {
        // Contained, outlined, and toolbar buttons are pill-shaped (GM3).
        root: {
          borderRadius: radii.pill,
          '&.Mui-focusVisible': {
            outline: `2px solid ${fleetColors.primary}`,
            outlineOffset: 2,
          },
        },
        // Text buttons (scorecard KPI tiles, inline links, dialog actions) use a
        // 6px rounded rectangle and shared tonal hover fill instead of a 9999px
        // pill that looks circular on tall or compact elements.
        text: {
          borderRadius: radii.sm,
          '&:hover': {
            backgroundColor: fleetColors.surfaceTonal,
          },
        },
      },
      variants: [
        {
          // Compact pill for table toolbars and page-header actions.
          // Usage: <Button variant="toolbar" startIcon={...}>Deploy</Button>
          // Neutral (non-primary) action: add color="inherit".
          props: { variant: 'toolbar' },
          style: {
            height: controlHeights.button,
            minHeight: controlHeights.button,
            padding: '0 12px',
            borderRadius: radii.pill,
            border: `1px solid ${fleetColors.borderPrimary}`,
            backgroundColor: fleetColors.canvas,
            color: fleetColors.primary,
            fontSize: fontSizes.label,
            fontWeight: 500,
            lineHeight: 1,
            whiteSpace: 'nowrap',
            '& .MuiButton-startIcon': { marginLeft: 0, marginRight: 6 },
            '& .MuiButton-startIcon > svg, & .MuiButton-endIcon > svg': {
              fontSize: iconSizes.control,
            },
            '&:hover': {
              backgroundColor: fleetColors.primaryContainer,
              borderColor: fleetColors.borderHover,
            },
            '&.Mui-focusVisible': {
              outline: `2px solid ${fleetColors.primary}`,
              outlineOffset: 2,
            },
            '&.Mui-disabled': {
              borderColor: fleetColors.border,
              backgroundColor: fleetColors.canvas,
            },
          },
        },
        {
          props: { variant: 'toolbar', color: 'inherit' },
          style: {
            borderColor: fleetColors.border,
            color: fleetColors.textPrimary,
            '&:hover': {
              backgroundColor: fleetColors.primaryContainer,
              borderColor: fleetColors.borderPrimary,
              color: fleetColors.primary,
            },
          },
        },
      ],
    },
    MuiChip: {
      defaultProps: {
        deleteIcon: <ClearIcon />,
      },
      styleOverrides: {
        root: {
          borderRadius: radii.pill,
          fontFamily: fontFamilies.text,
          '&.Mui-focusVisible': {
            outline: `2px solid ${fleetColors.primary}`,
            outlineOffset: 2,
          },
        },
      },
      variants: [
        {
          props: {
            variant: 'outlined',
          },
          style: {
            borderColor: fleetColors.border,
            // needs the class selector to be specific enough to override the default
            '&.MuiChip-clickable:hover, :focus': {
              backgroundColor: fleetColors.surfaceTonal,
            },
          },
        },
        {
          // Borderless tonal chip for applied filters.
          props: {
            variant: 'filter',
          },
          style: {
            backgroundColor: fleetColors.primaryContainer,
            color: fleetColors.primary,
            border: 'none',
            fontSize: fontSizes.label,
            fontWeight: 500,
            ':hover, :focus': {
              backgroundColor: fleetColors.primaryContainerHover,
              color: fleetColors.primary,
            },
            '.MuiChip-deleteIcon': {
              color: fleetColors.primary,
              borderRadius: '50%',
              ':hover, :focus': {
                color: fleetColors.primaryHover,
                backgroundColor: fleetColors.borderPrimary,
                transition: '0.2s',
              },
            },
          },
        },
      ],
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: { borderRadius: radii.sm },
      },
    },
    MuiPaper: {
      styleOverrides: {
        rounded: {
          borderRadius: radii.md,
        },
      },
      variants: [
        {
          props: { variant: 'surface' },
          style: {
            backgroundColor: fleetColors.surface,
            border: borders.default,
            borderRadius: radii.md,
            boxShadow: elevations.card,
          },
        },
      ],
    },
    MuiCard: {
      defaultProps: {
        elevation: 0,
      },
      styleOverrides: {
        root: {
          border: borders.default,
          boxShadow: elevations.card,
        },
      },
      variants: [
        {
          props: { variant: 'surface' },
          style: {
            backgroundColor: fleetColors.surface,
            border: borders.default,
            borderRadius: radii.md,
            boxShadow: elevations.card,
          },
        },
      ],
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          borderRadius: radii.sm,
          fontSize: fontSizes.caption,
        },
      },
    },
    MuiTabs: {
      styleOverrides: {
        root: {
          minHeight: controlHeights.tab,
        },
        indicator: {
          height: 3,
          borderTopLeftRadius: 3,
          borderTopRightRadius: 3,
          backgroundColor: fleetColors.primary,
        },
      },
    },
    MuiTab: {
      styleOverrides: {
        root: {
          minHeight: controlHeights.tab,
          padding: '8px 16px',
          fontFamily: fontFamilies.display,
          fontSize: fontSizes.body,
          fontWeight: 500,
          textTransform: 'none',
          color: fleetColors.textSecondary,
          '&:hover': {
            backgroundColor: fleetColors.surfaceTonal,
            color: fleetColors.textPrimary,
          },
          '&.Mui-focusVisible': {
            outline: `2px solid ${fleetColors.primary}`,
            outlineOffset: -2,
          },
          '&.Mui-selected': {
            color: fleetColors.primary,
          },
        },
      },
    },
    MuiMenu: {
      defaultProps: {
        elevation: 2,
      },
      styleOverrides: {
        list: {
          padding: '4px 0',
        },
      },
    },
    MuiMenuItem: {
      styleOverrides: {
        root: {
          minHeight: 36,
          fontSize: fontSizes.label,
          ':hover, :focus, :active': {
            backgroundColor: fleetColors.primaryContainer,
          },
        },
      },
    },
    MuiListItemIcon: {
      styleOverrides: {
        root: {
          minWidth: 32,
        },
      },
    },
  },
} satisfies DeepPartial<Theme>);

declare module '@mui/material/Button' {
  interface ButtonPropsVariantOverrides {
    toolbar: true;
  }
}

declare module '@mui/material/Chip' {
  interface ChipPropsVariantOverrides {
    filter: true;
  }
}

declare module '@mui/material/Paper' {
  interface PaperPropsVariantOverrides {
    surface: true;
  }
}

declare module '@mui/material/styles' {
  interface TypographyVariants {
    subhead1?: React.CSSProperties;
    subhead2?: React.CSSProperties;
  }

  // allow configuration using `createTheme()`
  interface TypographyVariantsOptions {
    subhead1?: React.CSSProperties;
    subhead2?: React.CSSProperties;
  }
}

// Update the Typography's variant prop options
declare module '@mui/material/Typography' {
  interface TypographyPropsVariantOverrides {
    subhead1: true;
    subhead2: true;
  }
}
