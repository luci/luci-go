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

const GOOGLE_SANS_FONT_FAMILY = "'Google Sans', 'Roboto', sans-serif";
const GOOGLE_SANS_TEXT_FONT_FAMILY =
  "'Google Sans Text', 'Google Sans', 'Roboto', sans-serif";

export const theme = createTheme(baseTheme, {
  palette: {
    text: {
      primary: colors.grey[900],
    },
    primary: {
      main: colors.blue[600],
      dark: colors.blue[800],
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
    fontFamily: 'Roboto',
    h1: {
      fontFamily: GOOGLE_SANS_FONT_FAMILY,
      fontSize: 36,
      lineHeight: '44px',
      fontWeight: 400,
    },
    h2: {
      fontFamily: GOOGLE_SANS_FONT_FAMILY,
      fontSize: 32,
      lineHeight: '40px',
      fontWeight: 400,
    },
    h3: { fontFamily: 'Roboto', fontSize: 28, lineHeight: '36px' },
    h4: {
      fontFamily: GOOGLE_SANS_FONT_FAMILY,
      fontSize: 24,
      lineHeight: '32px',
      fontWeight: 400,
    },
    h5: {
      fontFamily: GOOGLE_SANS_FONT_FAMILY,
      fontSize: 22,
      lineHeight: '28px',
      fontWeight: 400,
    },
    h6: {
      fontFamily: GOOGLE_SANS_FONT_FAMILY,
      fontSize: 18,
      lineHeight: '24px',
      fontWeight: 500,
    },
    subhead1: {
      fontFamily: GOOGLE_SANS_TEXT_FONT_FAMILY,
      fontSize: 16,
      lineHeight: '24px',
      fontWeight: 400,
    },
    subhead2: {
      fontFamily: 'Roboto',
      fontSize: 14,
      lineHeight: '24px',
      fontWeight: 400,
    },
    subtitle1: {
      fontFamily: GOOGLE_SANS_TEXT_FONT_FAMILY,
      fontSize: 16,
      lineHeight: '24px',
      fontWeight: 500,
    },
    subtitle2: {
      fontFamily: GOOGLE_SANS_TEXT_FONT_FAMILY,
      fontSize: 14,
      lineHeight: '20px',
      fontWeight: 500,
    },
    body1: { fontFamily: 'Roboto', fontSize: 16, lineHeight: '24px' },
    body2: { fontFamily: 'Roboto', fontSize: 14, lineHeight: '20px' },
    caption: { fontFamily: 'Roboto', fontSize: 12, lineHeight: '16px' },
    button: {
      fontFamily: 'Roboto',
      fontSize: 14,
      textTransform: 'none',
    },
  },
  components: {
    MuiChip: {
      defaultProps: {
        deleteIcon: <ClearIcon />,
      },
      variants: [
        {
          props: {
            variant: 'outlined',
          },
          style: {
            // needs the class selector to be specific enough to override the default
            '&.MuiChip-clickable:hover, :focus': {
              backgroundColor: colors.grey[100],
            },
          },
        },
        {
          props: {
            variant: 'filter',
          },
          style: {
            backgroundColor: colors.blue[50],
            color: colors.blue[600],
            fontSize: 14,
            ':hover, :focus': {
              backgroundColor: colors.blue[100],
              color: colors.blue[600],
            },
            '.MuiChip-deleteIcon': {
              color: colors.blue[600],
              borderRadius: '50%',
              ':hover, :focus': {
                color: colors.blue[600],
                backgroundColor: colors.blue[200],
                transition: '0.3s',
              },
            },
          },
        },
      ],
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
          ':hover, :focus, :active': {
            backgroundColor: colors.blue[50],
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

declare module '@mui/material/Chip' {
  interface ChipPropsVariantOverrides {
    filter: true;
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
