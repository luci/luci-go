// Copyright 2026 The LUCI Authors.
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

import { useMutation } from '@tanstack/react-query';

import {
  TokenType,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';
import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { useToast } from '@/crystal_ball/hooks';

/**
 * Arguments for exporting data to a Google Sheet.
 */
export interface ExportToSheetsArgs {
  title: string;
  values: (string | number | null | undefined)[][];
}

interface CreateSpreadsheetResponse {
  spreadsheetId: string;
  spreadsheetUrl: string;
}

function isCreateSpreadsheetResponse(
  obj: object,
): obj is CreateSpreadsheetResponse {
  return 'spreadsheetId' in obj && 'spreadsheetUrl' in obj;
}

const SHEETS_API_BASE = 'https://sheets.googleapis.com/v4/spreadsheets';

/**
 * Hook for exporting data to Google Sheets.
 */
export const useExportToSheets = () => {
  const getAccessToken = useGetAuthToken(TokenType.Access);
  const { showSuccessToast, showErrorToast } = useToast();

  return useMutation({
    mutationFn: async (args: ExportToSheetsArgs) => {
      const accessToken = await getAccessToken();
      gapi.client.setToken({ access_token: accessToken });

      // 1. Create spreadsheet
      const createResponse = await gapi.client.request({
        path: SHEETS_API_BASE,
        method: 'POST',
        body: {
          properties: {
            title: args.title,
          },
        },
      });
      const spreadsheet = JSON.parse(createResponse.body);

      if (!isCreateSpreadsheetResponse(spreadsheet)) {
        throw new Error('Invalid response from Sheets API');
      }

      const spreadsheetId = spreadsheet.spreadsheetId;
      const spreadsheetUrl = spreadsheet.spreadsheetUrl;

      // 2. Append values
      await gapi.client.request({
        path: `${SHEETS_API_BASE}/${spreadsheetId}/values/A1:append`,
        method: 'POST',
        params: {
          valueInputOption: 'USER_ENTERED',
        },
        body: {
          values: args.values,
        },
      });

      return { spreadsheetId, spreadsheetUrl };
    },
    onSuccess: (data) => {
      showSuccessToast(COMMON_MESSAGES.EXPORT_SUCCESS);
      if (data.spreadsheetUrl) {
        window.open(data.spreadsheetUrl, '_blank');
      }
    },
    onError: (error) => {
      showErrorToast(error, COMMON_MESSAGES.EXPORT_FAILED);
    },
  });
};
