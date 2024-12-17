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
    h1: { fontFamily: 'Google Sans', fontSize: 36, lineHeight: '44px' },
    h2: { fontFamily: 'Google Sans', fontSize: 32, lineHeight: '40px' },
    h3: { fontFamily: 'Google Sans', fontSize: 28, lineHeight: '36px' },
    h4: { fontFamily: 'Google Sans', fontSize: 24, lineHeight: '32px' },
    h5: { fontFamily: 'Google Sans', fontSize: 22, lineHeight: '28px' },
    h6: { fontFamily: 'Google Sans', fontSize: 18, lineHeight: '24px' },
    subhead1: { fontFamily: 'Google Sans', fontSize: 16, lineHeight: '24px' },
    subhead2: { fontFamily: 'Google Sans', fontSize: 14, lineHeight: '24px' },
    subtitle1: {
      fontFamily: 'Roboto Medium',
      fontSize: 16,
      lineHeight: '24px',
    },
    subtitle2: {
      fontFamily: 'Roboto Medium',
      fontSize: 14,
      lineHeight: '20px',
    },
    body1: { fontFamily: 'Roboto', fontSize: 16, lineHeight: '24px' },
    body2: { fontFamily: 'Roboto', fontSize: 14, lineHeight: '20px' },
    caption: { fontFamily: 'Roboto', fontSize: 12, lineHeight: '16px' },
    button: {
      fontFamily: 'Google Sans',
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
    MuiMenuItem: {
      styleOverrides: {
        root: {
          ':hover, :focus, :active': {
            backgroundColor: colors.blue[50],
          },
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
