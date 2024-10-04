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

import { HelpOutline } from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  InputAdornment,
  styled,
  Link,
} from '@mui/material';
import { isEqual } from 'lodash-es';
import { DateTime } from 'luxon';
import { useState, useRef } from 'react';
import { useLocation } from 'react-router-dom';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import {
  emptyPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import {
  getAbsoluteStartEndTime,
  TimeRangeSelector,
} from '@/common/components/time_range_selector';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Result } from '@/generic_libs/types';

import {
  PREFIX_MATCH_OPTION,
  EXACT_MATCH_OPTION,
  REGEX_CONTAIN_OPTION,
  EXACT_CONTAIN_OPTION,
} from './constants';
import {
  LogGroupListStateProvider,
  PaginationProvider,
  SearchFilter,
  SearchFilterProvider,
} from './context';
import { EMPTY_FORM, FormData } from './form_data';
import { SelectTextField } from './select_text_field';
import { TabLabel } from './tab_label';

const FormContainer = styled(Box)`
  margin: 10px;
  padding: 10px;
  border: solid 1px rgba(0, 0, 0, 0.23);
  border-radius: 5px;
`;

const FormRowDiv = styled(Box)`
  display: flex;
  gap: 5px;
  margin: 10px;
`;

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];
const DEFAULT_PAGE_SIZE = 10;

const validateTimeRange = (
  startTime: DateTime,
  endTime: DateTime,
): Result<'', string> => {
  if (startTime >= endTime) {
    return { ok: false, value: 'start time must be before end time' };
  }
  if (endTime.diff(startTime, ['days']).days > 7) {
    return {
      ok: false,
      value: 'start time should not be more than 7 days older than end time',
    };
  }
  return { ok: true, value: '' };
};

export function LogSearch() {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [pendingForm, setPendingForm] = useState<FormData>(
    FormData.fromSearchParam(searchParams) || EMPTY_FORM,
  );
  // Persist current time between re-render, so that anchor for relative time calculation is consistent.
  // This improve the cache hit rate of log search queries.
  const nowRef = useRef(DateTime.now().toUTC());
  const { startTime, endTime } = getAbsoluteStartEndTime(
    searchParams,
    nowRef.current,
  );

  // Basic Validation of the filters.
  // Filters passes this validation does NOT guarantee to be a valid
  // log search request. Backend does a more comprehensive validation.
  const isSubmittable = (() => {
    if (!startTime || !endTime) {
      return false;
    }
    if (!validateTimeRange(startTime, endTime).ok) {
      return false;
    }
    if (pendingForm.searchStr === '') {
      return false;
    }
    return true;
  })();
  // Prefill the search filter when data derived from URL parameter is submittable.
  // The page will start to query search results once user land this page (without click the search buttom).
  const initialFilter: SearchFilter | null = isSubmittable
    ? { form: { ...pendingForm }, startTime: startTime!, endTime: endTime! }
    : null;
  const [searchFilter, setSearchFilter] = useState<SearchFilter | null>(
    initialFilter,
  );
  const noUncommittedUpdate = isEqual(searchFilter, {
    form: { ...pendingForm },
    startTime: startTime,
    endTime: endTime,
  });
  const pagerContexts = {
    testLogPagerCtx: usePagerContext({
      pageSizeOptions: PAGE_SIZE_OPTIONS,
      defaultPageSize: DEFAULT_PAGE_SIZE,
      pageTokenKey: 'testLogCursor',
      // Use the same page size key for both test log and invocation log,
      // so that the the page limit is synchronized between both list.
      // If user decides to use a different page size in one tab, they probably want the same in another tab.
      pageSizeKey: 'limit',
    }),
    invocationLogPagerCtx: usePagerContext({
      pageSizeOptions: PAGE_SIZE_OPTIONS,
      defaultPageSize: DEFAULT_PAGE_SIZE,
      pageTokenKey: 'invLogCursor',
      pageSizeKey: 'limit',
    }),
  };
  return (
    <>
      <FormContainer>
        <FormRowDiv>
          <TimeRangeSelector
            validateCustomizeTimeRange={validateTimeRange}
            disableFuture
          />
          <SelectTextField
            required
            sx={{ flex: 3 }}
            label="Search string"
            selectValue={
              pendingForm.isSearchStrRegex
                ? REGEX_CONTAIN_OPTION
                : EXACT_CONTAIN_OPTION
            }
            textValue={pendingForm.searchStr}
            options={[REGEX_CONTAIN_OPTION, EXACT_CONTAIN_OPTION]}
            selectOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                isSearchStrRegex: str === REGEX_CONTAIN_OPTION,
              }));
            }}
            textOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                searchStr: str,
              }));
            }}
            endAdornment={
              <InputAdornment position="end">
                <HtmlTooltip
                  title={
                    <>
                      <strong>Contain</strong> - case-insensitive match
                      <br />
                      <br />
                      <strong>Regex</strong> - Regular expresssion support using
                      the{' '}
                      <Link
                        href="https://github.com/google/re2/wiki/Syntax"
                        target="_blank"
                        rel="noreferrer"
                      >
                        re2
                      </Link>{' '}
                      library with the following limitations.
                      <Box sx={{ marginLeft: '1em' }}>
                        <li>
                          Anchors are not supported. ^ and $ are treated as
                          literal characters
                        </li>
                        <li>Capture groups are not allowed.</li>
                        <li>
                          Flags are not supported. All flags are set to default.
                        </li>
                      </Box>
                      <br />
                      See more details about this page in{' '}
                      <Link
                        href="http://go/luci-guide-log-search"
                        target="_blank"
                        rel="noreferrer"
                      >
                        go/luci-guide-log-search
                      </Link>
                      .
                    </>
                  }
                >
                  <HelpOutline fontSize="small" />
                </HtmlTooltip>
              </InputAdornment>
            }
          />
        </FormRowDiv>
        <FormRowDiv>
          <SelectTextField
            sx={{ flex: 3 }}
            label="Test ID"
            selectValue={
              pendingForm.isTestIDStrPrefix
                ? PREFIX_MATCH_OPTION
                : EXACT_MATCH_OPTION
            }
            textValue={pendingForm.testIDStr}
            options={[EXACT_MATCH_OPTION, PREFIX_MATCH_OPTION]}
            selectOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                isTestIDStrPrefix: str === PREFIX_MATCH_OPTION,
              }));
            }}
            textOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                testIDStr: str,
              }));
            }}
            endAdornment={
              <InputAdornment position="end">
                <HtmlTooltip
                  title={
                    <>
                      What is a Test ID? see{' '}
                      <Link
                        href="http://go/luci-guide-log-search#step3-find-the-test-id-log-file-of-the-failure"
                        target="_blank"
                        rel="noreferrer"
                      >
                        go/luci-guide-log-search
                      </Link>
                      .
                    </>
                  }
                >
                  <HelpOutline fontSize="small" />
                </HtmlTooltip>
              </InputAdornment>
            }
          />
          <SelectTextField
            sx={{ flex: 2 }}
            label="Log file"
            selectValue={
              pendingForm.isArtifactIDStrPrefix
                ? PREFIX_MATCH_OPTION
                : EXACT_MATCH_OPTION
            }
            textValue={pendingForm.artifactIDStr}
            options={[EXACT_MATCH_OPTION, PREFIX_MATCH_OPTION]}
            selectOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                isArtifactIDStrPrefix: str === PREFIX_MATCH_OPTION,
              }));
            }}
            textOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                artifactIDStr: str,
              }));
            }}
            endAdornment={
              <InputAdornment position="end">
                <HtmlTooltip
                  title={
                    <>
                      What is a log file? see{' '}
                      <Link
                        href="http://go/luci-guide-log-search#step3-find-the-test-id-log-file-of-the-failure"
                        target="_blank"
                        rel="noreferrer"
                      >
                        go/luci-guide-log-search
                      </Link>
                      .
                    </>
                  }
                >
                  <HelpOutline fontSize="small" />
                </HtmlTooltip>
              </InputAdornment>
            }
          />
        </FormRowDiv>
        <Button
          sx={{ margin: '0px 10px' }}
          size="small"
          variant="contained"
          disabled={!isSubmittable || noUncommittedUpdate}
          data-testid="search-button"
          onClick={() => {
            if (!isSubmittable) {
              return;
            }
            setSearchFilter({
              form: { ...pendingForm },
              startTime: startTime!,
              endTime: endTime!,
            });
            setSearchParams(FormData.toSearchParamUpdater(pendingForm));
            setSearchParams(
              emptyPageTokenUpdater(pagerContexts.invocationLogPagerCtx),
            );
            setSearchParams(
              emptyPageTokenUpdater(pagerContexts.testLogPagerCtx),
            );
          }}
        >
          {searchFilter ? 'update search' : 'search'}
        </Button>
      </FormContainer>
      {pendingForm.artifactIDStr === '' && pendingForm.testIDStr === '' && (
        <Alert severity="warning">
          Query might be slow without a<strong> test ID filter</strong> or{' '}
          <strong>log file filter</strong>.
        </Alert>
      )}
      <SearchFilterProvider searchFilter={searchFilter}>
        <PaginationProvider state={pagerContexts}>
          <LogGroupListStateProvider>
            <AppRoutedTabs sx={{ display: searchFilter ? '' : 'none' }}>
              <AppRoutedTab
                label={
                  <TabLabel
                    label={'Test result logs'}
                    tooltipText={`Test result logs are directly associated with a
                          specific test result. They are text files produced by
                          a test that provide detailed information about the
                          test's execution.`}
                  />
                }
                value="test-logs"
                to={`test-logs${location.search}`}
              />
              <AppRoutedTab
                label={
                  <TabLabel
                    label={'Shared logs'}
                    tooltipText={`Shared logs are text files that are associated with a
                          group of test results, providing shared information
                          about these test results.`}
                  />
                }
                value="shared-logs"
                to={`shared-logs${location.search}`}
              />
            </AppRoutedTabs>
          </LogGroupListStateProvider>
        </PaginationProvider>
      </SearchFilterProvider>
    </>
  );
}
