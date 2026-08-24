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
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import {
  Button,
  Link,
  Menu,
  MenuItem,
  Snackbar,
  Typography,
} from '@mui/material';
import copy from 'copy-to-clipboard';
import { useMemo, useState } from 'react';

import { InstructionDialog } from '@/common/components/instruction_hint/instruction_dialog';
import { pairsToPlaceholderDict } from '@/common/tools/instruction/instruction_utils';
import { useInvocation, useTestVariant } from '@/test_investigation/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

import {
  isAnTSInvocation,
  getInvocationTag,
} from '../../../utils/test_info_utils';

import {
  AndroidBuild,
  SAFE_BRANCH_REGEX,
  SAFE_TARGET_REGEX,
  SAFE_BUILD_ID_REGEX,
  isVirtualTarget,
  getAtestCommand,
  getForrestLink,
} from './rerun_utils';

export function RerunButton() {
  const testVariant = useTestVariant();
  const invocation = useInvocation();

  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const isAnTS = useMemo(() => isAnTSInvocation(invocation), [invocation]);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const menuOpen = Boolean(anchorEl);

  const instructionPlaceHolderData = useMemo(() => {
    const resultBundle = testVariant.results || [];
    const tagDict = pairsToPlaceholderDict(resultBundle[0].result.tags);
    return {
      test: {
        id: testVariant.testId,
        metadata: testVariant.testMetadata || {},
        variant: testVariant.variant?.def || {},
        tags: tagDict,
      },
    };
  }, [
    testVariant.results,
    testVariant.testId,
    testVariant.testMetadata,
    testVariant.variant?.def,
  ]);

  const handleClickMenu = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };
  const handleCloseMenu = () => {
    setAnchorEl(null);
  };

  /**
   * Determine whether to show the atest command. This is used for providing
   * additional derived atest commands such as the command with acloud.
   */
  const showAtestRerun = useMemo(() => {
    if (getInvocationTag(invocation.tags, 'local_test_runner') === 'atest') {
      return true;
    }
    return false;
  }, [invocation.tags]);

  // TODO(b/445559255): add logic here for when module level page can be shown. For now, always show it.
  /**
   * Determines whether to show the atest command for a module.
   *
   * If the test result is not at the module level, the command would be
   * different than the normal atest command, so show it.
   */
  const showAtestModuleRerun = useMemo(() => {
    return true;
  }, []);

  /**
   * Determines whether to show the atest command for acloud. If the target is a
   * virtual target, then show the command.
   */
  const showAtestAcloudRerun = () => {
    if (!showAtestRerun) {
      return false;
    }
    const primaryBuild: AndroidBuild | undefined = isRootInvocation(invocation)
      ? invocation?.primaryBuild?.androidBuild
      : invocation?.properties?.primaryBuild;
    if (
      !primaryBuild?.branch ||
      !primaryBuild?.buildTarget ||
      !primaryBuild?.buildId ||
      !SAFE_BRANCH_REGEX.test(primaryBuild.branch) ||
      !SAFE_TARGET_REGEX.test(primaryBuild.buildTarget) ||
      !SAFE_BUILD_ID_REGEX.test(primaryBuild.buildId)
    ) {
      return false;
    }
    return isVirtualTarget(primaryBuild.buildTarget);
  };

  /**
   * Determine whether to show the ABTD atest command.
   * The test is restricted to:
   *   branch: git_main
   *   targets: cuttlefish, oriole, husky
   *   test mapping
   */
  const showAtestAbtdRerun = () => {
    const primaryBuild: AndroidBuild = isRootInvocation(invocation)
      ? invocation?.primaryBuild?.androidBuild
      : invocation?.properties?.primaryBuild;

    const isValidBranch = primaryBuild?.branch === 'git_main';

    const VALID_TARGETS = ['cf', 'oriole', 'husky'];
    const target = primaryBuild?.buildTarget ?? '';
    const isValidTarget = VALID_TARGETS.some((t) => target.includes(t));

    const hasTestMappingSource = testVariant?.results[0].result.tags?.find(
      (tag) => tag.key === 'test_mapping_source',
    )?.value;
    const testName = invocation.properties?.testDefinition?.name ?? '';
    const hasTestMappingName =
      testName.includes('test-mapping') || testName.includes('test_mapping');

    return (
      isValidBranch &&
      isValidTarget &&
      (hasTestMappingSource || hasTestMappingName)
    );
  };

  const copyRerunTest = () => {
    const text = getAtestCommand(testVariant);
    if (text) {
      copy(text);
      setSnackbarOpen(true);
    }
    handleCloseMenu();
  };

  const copyRerunModule = () => {
    const text = getAtestCommand(testVariant, { moduleOnly: true });
    if (text) {
      copy(text);
      setSnackbarOpen(true);
    }
    handleCloseMenu();
  };

  const copyAcloudCommand = () => {
    const primaryBuild: AndroidBuild = isRootInvocation(invocation)
      ? invocation?.primaryBuild?.androidBuild
      : invocation?.properties?.primaryBuild;
    const text = getAtestCommand(testVariant, { acloudBuild: primaryBuild });
    if (text) {
      copy(text);
      setSnackbarOpen(true);
    }
    handleCloseMenu();
  };

  /** Generate the rerun command for atest via ABTD. */
  const getAbtdAtestLink = () => {
    if (!testVariant) {
      return '';
    }
    const command = getAtestCommand(testVariant, {
      omitAtest: true,
      omitExtraArgs: true,
    });
    return getForrestLink(invocation, command ?? undefined);
  };

  // TODO(b/445559255): conditionally render 'test case' or 'module' in rerun button labels when module page available.
  return (
    <>
      {isAnTS ? (
        <>
          <Button
            variant="outlined"
            size="small"
            endIcon={<ArrowDropDownIcon sx={{ p: 0, m: 0 }} />}
            aria-controls={menuOpen ? 'rerun-menu' : undefined}
            aria-haspopup="true"
            aria-expanded={menuOpen ? 'true' : undefined}
            data-testid="rerun-dropdown"
            onClick={handleClickMenu}
          >
            Rerun
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
            {showAtestRerun ? (
              <MenuItem onClick={copyRerunTest}>
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: 'primary.main' }}
                >
                  Rerun test case with atest
                </Typography>
              </MenuItem>
            ) : (
              <MenuItem
                href="http://go/android-test-investigate-docs#common-debug-actions"
                target="_blank"
                rel="noreferrer"
                component={Link}
              >
                <Typography variant="body2" sx={{ color: 'text.disabled' }}>
                  Rerun test case locally
                </Typography>
                <HelpOutlineIcon sx={{ m: 0, p: 0, color: 'text.disabled' }} />
              </MenuItem>
            )}
            {showAtestAbtdRerun() && (
              <MenuItem
                component={Link}
                href={getAbtdAtestLink()}
                target="_blank"
              >
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: 'primary.main' }}
                >
                  Rerun test case with atest in ABTD
                </Typography>
              </MenuItem>
            )}
            {showAtestModuleRerun && (
              <MenuItem onClick={copyRerunModule}>
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: 'primary.main' }}
                >
                  Rerun full module with atest
                </Typography>
              </MenuItem>
            )}
            {showAtestAcloudRerun() && (
              <MenuItem onClick={copyAcloudCommand}>
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: 'primary.main' }}
                >
                  Rerun test case with atest in acloud
                </Typography>
              </MenuItem>
            )}
          </Menu>
          <Snackbar
            open={snackbarOpen}
            autoHideDuration={3000}
            onClose={() => setSnackbarOpen(false)}
            message="atest command copied"
          />
        </>
      ) : (
        <Button
          data-testid="rerun-button"
          variant="outlined"
          size="small"
          onClick={() => {
            setDialogOpen(true);
          }}
        >
          Rerun
        </Button>
      )}
      {testVariant.instruction?.instruction && (
        <InstructionDialog
          open={dialogOpen}
          onClose={(e) => {
            setDialogOpen(false);
            e.stopPropagation();
          }}
          title={'rerun instructions'}
          instructionName={testVariant.instruction?.instruction}
          placeholderData={instructionPlaceHolderData}
        />
      )}
    </>
  );
}
