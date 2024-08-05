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

import {
  emptyPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { TimeRangeSelector } from '@/common/components/time_range_selector';
import { getAbsoluteStartEndTime } from '@/common/components/time_range_selector';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  PREFIX_MATCH_OPTION,
  EXACT_MATCH_OPTION,
  REGEX_MATCH_OPTION,
} from './constants';
import { FormData, CompleteFormToSearch } from './form_data';
import { LogListDialog } from './log_list_dialog';
import { LogTable } from './log_table';
import { LogGroupListStateProvider } from './providers';
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

function searchParamUpdater(newFormData: FormData) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    // TODO (beining@): Using JSON.stringify for the search parameter is backward incompatible
    // if any property is renamed. A better way is to have a constant search parameter key for each
    // of these field.
    searchParams.set('filter', JSON.stringify(newFormData));
    return searchParams;
  };
}

const emptyForm: FormData = {
  testIDStr: '',
  isTestIDStrPrefix: false,
  artifactIDStr: '',
  isArtifactIDStrPrefix: false,
  searchStr: '',
  isSearchStrRegex: false,
};

function parseSearchParam(searchParams: URLSearchParams): FormData {
  const urlParam: FormData = JSON.parse(searchParams.get('filter') || '{}');
  return {
    ...emptyForm,
    ...urlParam,
  };
}

export interface LogSearchProps {
  readonly project: string;
}

// TODO(@beining) :
// * implement some validation before sending request.
export function LogSearch({ project }: LogSearchProps) {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 20, 50, 100],
    defaultPageSize: 10,
  });
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [pendingForm, setPendingForm] = useState<FormData>(
    parseSearchParam(searchParams),
  );
  // formToSearch is not initialized with filters from the URL parameters.
  // This is because we only want to initiate a search when user click the search button.
  // Search can be expensive, we should encourage checking the filters before searching.
  const [formToSearch, setFormToSearch] = useState<CompleteFormToSearch>();
  // Persist current time between re-render, so that anchor for relative time calculation is consistent.
  const nowRef = useRef(DateTime.now().toUTC());
  const { startTime, endTime } = getAbsoluteStartEndTime(
    searchParams,
    nowRef.current,
  );
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
            setFormToSearch({ ...pendingForm, startTime, endTime });
            setSearchParams(searchParamUpdater(pendingForm));
            setSearchParams(emptyPageTokenUpdater(pagerCtx));
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
      <LogGroupListStateProvider>
        {formToSearch && (
          <>
            <LogTable
              project={project}
              pagerCtx={pagerCtx}
              form={formToSearch}
            />
            <LogListDialog project={project} form={formToSearch} />
          </>
        )}
      </LogGroupListStateProvider>
    </>
  );
}
