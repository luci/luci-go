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
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { DateTime } from 'luxon';
import { useState } from 'react';

import {
  emptyPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  PREFIX_MATCH_OPTION,
  EXACT_MATCH_OPTION,
  REGEX_MATCH_OPTION,
} from './constants';
import { FormData } from './form_data';
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

interface searchUrlParam extends Omit<FormData, 'startTime' | 'endTime'> {
  readonly startTime?: string;
  readonly endTime?: string;
}

function searchParamUpdater(newFormData: FormData) {
  const searchParams = new URLSearchParams();
  const sp: searchUrlParam = {
    ...newFormData,
    startTime: newFormData.startTime?.toISO(),
    endTime: newFormData.endTime?.toISO(),
  };
  searchParams.set('filter', JSON.stringify(sp));
  return searchParams;
}

const emptyForm: FormData = {
  testIDStr: '',
  isTestIDStrPrefix: false,
  artifactIDStr: '',
  isArtifactIDStrPrefix: false,
  searchStr: '',
  isSearchStrRegex: true,
  startTime: null,
  endTime: null,
};

function parseSearchParam(searchParams: URLSearchParams): FormData {
  const urlParam: searchUrlParam = JSON.parse(
    searchParams.get('filter') || '{}',
  );
  const currentTime = DateTime.now().toUTC();
  const threeDaysAgo = currentTime.minus({ days: 3 });
  return {
    ...emptyForm,
    ...urlParam,
    startTime: urlParam.startTime
      ? DateTime.fromISO(urlParam.startTime).toUTC()
      : threeDaysAgo,
    endTime: urlParam.endTime
      ? DateTime.fromISO(urlParam.endTime).toUTC()
      : currentTime,
  };
}

export interface LogSearchProps {
  readonly project: string;
}

// TODO(@beining) :
// * implement some validation before sending request.
// * improve date range selection, eg quick select last 3 days, 5 days. 7 days.
export function LogSearch({ project }: LogSearchProps) {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 20, 50, 100],
    defaultPageSize: 10,
  });
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [pendingForm, setPendingForm] = useState<FormData>(
    parseSearchParam(searchParams),
  );
  const [formToSearch, setFormToSearch] = useState<FormData>();

  return (
    <>
      <FormContainer>
        <FormRowDiv>
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

          <DateTimePicker
            sx={{ flex: 1 }}
            label="From (UTC)"
            timezone="UTC"
            value={pendingForm.startTime}
            slotProps={{ textField: { size: 'small' } }}
            onChange={(newValue) =>
              setPendingForm((prev) => ({
                ...prev,
                startTime: newValue?.startOf('minute') || null,
              }))
            }
          />
          <DateTimePicker
            sx={{ flex: 1 }}
            label="To (UTC)"
            timezone="UTC"
            value={pendingForm.endTime}
            slotProps={{ textField: { size: 'small' } }}
            onChange={(newValue) =>
              setPendingForm((prev) => ({
                ...prev,
                endTime: newValue?.startOf('minute') || null,
              }))
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
            setFormToSearch({ ...pendingForm });
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
