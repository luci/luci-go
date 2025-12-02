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
  Box,
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
import { OutputTestVerdict } from '@/common/types/verdict';
import { useInvocation, useTestVariant } from '@/test_investigation/context';

import {
  isAnTSInvocation,
  getInvocationTag,
} from '../../../utils/test_info_utils';

function getAtestCommand(
  testVariant: OutputTestVerdict,
  params?: {
    moduleOnly?: boolean;
    omitAtest?: boolean;
    omitExtraArgs?: boolean;
  },
): string | null {
  const moduleName = testVariant?.testIdStructured?.moduleName || undefined;
  const testIdStructured = testVariant?.testIdStructured || undefined;
  if (moduleName === undefined) {
    return null;
  }
  let command = `${(params?.omitAtest ?? false) ? '' : 'atest '}${moduleName}`;
  if (!params?.moduleOnly) {
    const testClass = `${testIdStructured?.coarseName ?? ''}.${testIdStructured?.fineName ?? ''}`;
    const testMethod = testIdStructured?.caseName ?? '';
    if (testClass !== '' || testMethod !== '') {
      if (testClass !== '') {
        command = `${command}:${testClass}`;
      }
      if (testMethod !== '') {
        command = `${command}#${testMethod}`;
      }
    }
  }
  if (!params?.omitExtraArgs) {
    const extraArgs: string[] = [];
    const moduleAbi = testVariant.variant?.def['module_abi'];
    if (moduleAbi) {
      extraArgs.push(`--abi ${moduleAbi}`);
    }
    if (testVariant.variant?.def['module_param'] === 'instant') {
      extraArgs.push('--instant');
    }
    if (extraArgs.length > 0) {
      command = `${command} -- ${extraArgs.join(' ')}`;
    }
  }
  return command;
}

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

  // TODO(b/445811111): Add this functionality when invocation data has been migrated. For now, don't show the button option.
  /**
   * Determines whether to show the atest command for acloud. If the target is a
   * virtual target, then show the command.
   */
  const showAtestAcloudRerun = () => {
    return false;
  };

  const copyRerunTest = () => {
    const text = getAtestCommand(testVariant);
    if (text) {
      copy(text);
    }
    setSnackbarOpen(true);
    handleCloseMenu();
  };

  const copyRerunModule = () => {
    const text = getAtestCommand(testVariant, { moduleOnly: true });
    if (text) {
      copy(text);
    }
    setSnackbarOpen(true);
    handleCloseMenu();
  };

  // TODO(b/445559255): conditionally render 'test case' or 'module' in rerun button labels when module page available.
  return (
    <>
      {isAnTS ? (
        showAtestRerun ? (
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
              <MenuItem onClick={copyRerunTest}>
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 'bold', color: '#1A73E8' }}
                >
                  Rerun test case with atest
                </Typography>
              </MenuItem>
              {showAtestModuleRerun && (
                <MenuItem onClick={copyRerunModule}>
                  <Typography
                    variant="body2"
                    sx={{ fontWeight: 'bold', color: '#1A73E8' }}
                  >
                    Rerun full module with atest
                  </Typography>
                </MenuItem>
              )}
              {showAtestAcloudRerun() && (
                <MenuItem>
                  <Typography
                    variant="body2"
                    sx={{ fontWeight: 'bold', color: '#1A73E8' }}
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
          <Box
            sx={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}
          >
            <Button
              href="http://go/android-test-investigate-docs#common-debug-actions"
              target="_blank"
              rel="noreferrer"
              component={Link}
              size="small"
              variant="outlined"
              sx={{
                textDecoration: 'none',
                color: '#bdbdbd',
              }}
            >
              Rerun test case locally
              <HelpOutlineIcon
                sx={{ m: 0, p: 0 }}
                style={{ color: '#bdbdbd' }}
              />
            </Button>
          </Box>
        )
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
