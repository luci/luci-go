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

import { Tooltip, TooltipProps, styled, tooltipClasses } from '@mui/material';

// TODO(weiweilin): posible optimization.  Delete this code if not needed.
// export const HtmlTooltip = ({ children, ...props }: TooltipProps) => {
//   const [render, setRender] = useState(false);
//   return (
//     <div onMouseEnter={() => !render && setRender(true)}>
//       {!render && children}
//       {render && (
//         <HtmlTooltipComponent {...props}>{children}</HtmlTooltipComponent>
//       )}
//     </div>
//   );
// };

export const HtmlTooltip = styled(({ className, ...props }: TooltipProps) => (
  <Tooltip arrow {...props} classes={{ popper: className }} />
))(({ theme }) => ({
  [`& .${tooltipClasses.arrow}`]: {
    color: theme.palette.text.primary,
    '&::before': {
      backgroundColor: theme.palette.background.default,
      border: `1px solid ${theme.palette.divider}`,
    },
  },
  [`& .${tooltipClasses.tooltip}`]: {
    backgroundColor: theme.palette.background.default,
    color: theme.palette.text.primary,
    maxWidth: 600,
    fontWeight: 400,
    fontSize: '0.9rem',
    border: `1px solid ${theme.palette.divider}`,
    boxShadow: theme.shadows[4],
  },
}));
