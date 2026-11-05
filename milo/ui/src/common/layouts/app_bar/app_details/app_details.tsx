// Copyright 2023 The LUCI Authors.
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

import MenuIcon from '@mui/icons-material/Menu';
import Box from '@mui/material/Box';
import { Link as RouterLink } from 'react-router';

import {
  StyledIconButton,
  StyledVerticalDivider,
  StyledAppBarText,
  StyledAppBarLink,
} from '@/common/components/gm3_styled_components';
import { useActivePageId, useProjectCtx } from '@/common/components/page_meta';
import { getProjectURLPath } from '@/common/tools/url_utils';

import { PAGE_LABEL_MAP } from '../../constants';

/**
 * The default app name displayed in the app bar.
 */
export const DEFAULT_APP_NAME = 'LUCI';

/**
 * The default link target of the app name in the app bar.
 */
export const DEFAULT_APP_HOME_LINK = '/ui/';

interface Props {
  open: boolean;
  handleSidebarChanged: (isOpen: boolean) => void;
  showSidebarToggle?: boolean;
  /**
   * The app name displayed in the app bar. Defaults to `LUCI`.
   */
  appName?: string;
  /**
   * The link target of the app name. Defaults to `/ui/`.
   */
  appHomeLink?: string;
  /**
   * Hides the LUCI project breadcrumb. Defaults to `false`.
   */
  hideProject?: boolean;
}

export const AppDetails = ({
  open,
  handleSidebarChanged,
  showSidebarToggle = true,
  appName = DEFAULT_APP_NAME,
  appHomeLink = DEFAULT_APP_HOME_LINK,
  hideProject = false,
}: Props) => {
  const activePage = useActivePageId();
  const project = useProjectCtx();
  return (
    <>
      {showSidebarToggle && (
        <StyledIconButton
          aria-label="menu"
          onClick={() => handleSidebarChanged(!open)}
          edge={false}
          size="medium"
        >
          <MenuIcon />
        </StyledIconButton>
      )}
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <StyledAppBarLink
          component={RouterLink}
          to={appHomeLink}
          underline="none"
          sx={{ pr: 2, pl: 2 }}
        >
          {appName}
        </StyledAppBarLink>
        {!hideProject && project && (
          <>
            <StyledVerticalDivider
              flexItem
              orientation="vertical"
              sx={{ ml: 2, mr: 2 }}
            />
            <StyledAppBarLink
              component={RouterLink}
              to={getProjectURLPath(project)}
              // Make it obvious that this link is clickable during the
              // transition phase of redesigned build page top bar.
              // We can decide whether we want to keep this style later.
              underline="always"
              sx={{
                pr: 2,
                textDecorationColor: 'currentColor',
                '&:hover': {},
              }}
            >
              {project}
            </StyledAppBarLink>
          </>
        )}
        {activePage && PAGE_LABEL_MAP[activePage] && (
          <>
            <StyledVerticalDivider
              flexItem
              orientation="vertical"
              sx={{ ml: 2, mr: 2 }} // Apply margins via sx
            />
            {/* Use StyledAppBarText for plain text Page Title */}
            <StyledAppBarText
              component="div"
              sx={{ pr: 2 }} // Keep sx for spacing
            >
              {PAGE_LABEL_MAP[activePage]}
            </StyledAppBarText>
          </>
        )}
      </Box>
    </>
  );
};
