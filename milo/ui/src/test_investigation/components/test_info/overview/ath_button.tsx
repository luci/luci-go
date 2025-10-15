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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import { Button, Link, Menu, MenuItem, Typography } from '@mui/material';
import { useMemo, useState } from 'react';

import { useInvocation, useTestVariant } from '@/test_investigation/context';
import {
  getFullMethodName,
  isAnTSInvocation,
  getAndroidTestHubUrl,
} from '@/test_investigation/utils/test_info_utils';

/**
 * Symbols that overlap with ATH search query that should be escaped with
 * quotes.
 */
const ESCAPED_METHOD_SYMBOLS = new RegExp(/([\(\)\s])/);

export function AthButton() {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const menuOpen = Boolean(anchorEl);
  const invocation = useInvocation();
  const testVariant = useTestVariant();

  const handleClickMenu = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };
  const handleCloseMenu = () => {
    setAnchorEl(null);
  };

  const isAnTS = useMemo(
    () => isAnTSInvocation(invocation.name),
    [invocation.name],
  );

  /**
   * Generate the ATH urls for viewing test method runs across builds with options across targets and branch.
   */
  function getAthUrls(): {
    forTestUrl: string;
    forTargetUrl: string;
    forBranchUrl: string;
    forBranchTargetUrl: string;
  } {
    const primaryBuild = invocation?.properties?.primaryBuild;
    const target = primaryBuild?.buildTarget;
    const branch = primaryBuild?.branch;
    const methodName = getFullMethodName(testVariant);

    const urls = {
      forTestUrl: '',
      forTargetUrl: '',
      forBranchUrl: '',
      forBranchTargetUrl: '',
    };

    let testNameQuery = '';
    if (methodName === '') {
      return urls;
    } else {
      if (ESCAPED_METHOD_SYMBOLS.test(methodName)) {
        testNameQuery = `test:"${methodName}"`;
      } else {
        testNameQuery = `test:${methodName}`;
      }
      urls.forTestUrl = getAndroidTestHubUrl({ query: testNameQuery });
    }

    if (target !== null) {
      const query = `target:${target} ${testNameQuery}`;
      urls.forTargetUrl = getAndroidTestHubUrl({ query });
    }
    if (branch !== null) {
      const query = `branch:${branch} ${testNameQuery}`;
      urls.forBranchUrl = getAndroidTestHubUrl({ query });
    }
    if (branch !== null && target !== null) {
      const query = `branch:${branch} target:${target} ${testNameQuery}`;
      urls.forBranchTargetUrl = getAndroidTestHubUrl({ query });
    }

    return urls;
  }

  const urls = getAthUrls();
  const showAthButton = urls.forTestUrl !== '';

  const showForBranch = urls.forBranchUrl !== '';
  const showForTarget = urls.forTargetUrl !== '';
  const showForBranchTarget = urls.forBranchTargetUrl !== '';

  return (
    <>
      {showAthButton && isAnTS && (
        <>
          <Button
            variant="outlined"
            size="small"
            endIcon={<ArrowDropDownIcon sx={{ p: 0, m: 0 }} />}
            aria-controls={menuOpen ? 'ath-menu' : undefined}
            aria-haspopup="true"
            aria-expanded={menuOpen ? 'true' : undefined}
            data-testid="ath-dropdown"
            onClick={handleClickMenu}
          >
            View in ATH
          </Button>
          <Menu
            id="basic-menu"
            anchorEl={anchorEl}
            open={menuOpen}
            onClose={handleCloseMenu}
            slotProps={{
              list: {
                'aria-labelledby': 'basic-button',
              },
            }}
          >
            <MenuItem component={Link} href={urls.forTestUrl} target="_blank">
              <Typography
                variant="body2"
                sx={{ fontWeight: 'bold', color: '#1A73E8' }}
              >
                View test case in ATH
              </Typography>
            </MenuItem>
            {showForBranch && (
              <MenuItem
                component={Link}
                href={urls.forBranchUrl}
                target="_blank"
              >
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: '#1A73E8' }}
                >
                  View test for branch in ATH
                </Typography>
              </MenuItem>
            )}
            {showForTarget && (
              <MenuItem
                component={Link}
                href={urls.forTargetUrl}
                target="_blank"
              >
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: '#1A73E8' }}
                >
                  View test for target in ATH
                </Typography>
              </MenuItem>
            )}
            {showForBranchTarget && (
              <MenuItem
                component={Link}
                href={urls.forBranchTargetUrl}
                target="_blank"
              >
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: '#1A73E8' }}
                >
                  View test for branch and target in ATH
                </Typography>
              </MenuItem>
            )}
          </Menu>
        </>
      )}
    </>
  );
}
