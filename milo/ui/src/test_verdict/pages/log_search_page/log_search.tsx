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

import { Alert, Box, Button, styled } from '@mui/material';
import { DateTime } from 'luxon';
import { useState, useRef } from 'react';
import { useLocation } from 'react-router-dom';

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

import {
  PREFIX_MATCH_OPTION,
  EXACT_MATCH_OPTION,
  REGEX_MATCH_OPTION,
} from './constants';
import { SearchFilter } from './contexts';
import { EMPTY_FORM, FormData } from './form_data';
import {
  LogGroupListStateProvider,
  PaginationProvider,
  SearchFilterProvider,
} from './providers';
import { SelectTextField } from './select_text_field';
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

// TODO(@beining) :
// * implement some validation before sending request.
export function LogSearch() {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [pendingForm, setPendingForm] = useState<FormData>(
    FormData.fromSearchParam(searchParams) || EMPTY_FORM,
  );
  const [searchFilter, setSearchFilter] = useState<SearchFilter | null>(null);
  // Persist current time between re-render, so that anchor for relative time calculation is consistent.
  // This improve the cache hit rate of log search queries.
  const nowRef = useRef(DateTime.now().toUTC());
  const { startTime, endTime } = getAbsoluteStartEndTime(
    searchParams,
    nowRef.current,
  );
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
          <TimeRangeSelector />
          <SelectTextField
            sx={{ flex: 3 }}
            label="Search string"
            selectValue={
              pendingForm.isSearchStrRegex
                ? REGEX_MATCH_OPTION
                : EXACT_MATCH_OPTION
            }
            textValue={pendingForm.searchStr}
            options={[REGEX_MATCH_OPTION, EXACT_MATCH_OPTION]}
            selectOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                isSearchStrRegex: str === REGEX_MATCH_OPTION,
              }));
            }}
            textOnChange={(str) => {
              setPendingForm((prev) => ({
                ...prev,
                searchStr: str,
              }));
            }}
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
          />
        </FormRowDiv>
        <Button
          sx={{ margin: '0px 10px' }}
          size="small"
          variant="contained"
          onClick={() => {
            startTime &&
              endTime &&
              setSearchFilter({
                form: { ...pendingForm },
                startTime,
                endTime,
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
          Search
        </Button>
      </FormContainer>
      {pendingForm.artifactIDStr === '' && pendingForm.testIDStr === '' && (
        <Alert severity="warning">
          Query will be slow without an test ID filter or log file filter.
        </Alert>
      )}
      <SearchFilterProvider searchFilter={searchFilter}>
        <PaginationProvider state={pagerContexts}>
          <LogGroupListStateProvider>
            <AppRoutedTabs>
              <AppRoutedTab
                label="Test result logs"
                value="test-logs"
                to={`test-logs${location.search}`}
              />
              <AppRoutedTab
                label="Shared logs"
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
