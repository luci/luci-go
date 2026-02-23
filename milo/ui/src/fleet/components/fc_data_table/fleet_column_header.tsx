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

import { Typography } from '@mui/material';
import { MRT_Header, MRT_RowData } from 'material-react-table';
import { ReactNode } from 'react';

import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';

export interface FleetColumnHeaderProps<TData extends MRT_RowData> {
  header: MRT_Header<TData>;
  headerText?: string;
  infoTooltip?: ReactNode;
}

const HEADER_TEXT_STYLE = {
  fontWeight: 500,
  display: '-webkit-box',
  WebkitLineClamp: 2,
  WebkitBoxOrient: 'vertical' as const,
  overflow: 'hidden',
};

export interface FleetColumnHeaderContentProps {
  text: string;
  tooltip?: ReactNode;
}

export const FleetColumnHeaderContent = ({
  text,
  tooltip,
}: FleetColumnHeaderContentProps) => {
  return (
    <div
      css={{
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
      }}
    >
      <Typography
        variant="subhead2"
        title={text}
        css={{ ...HEADER_TEXT_STYLE, flex: 1, minWidth: 0 }}
      >
        {text}
      </Typography>

      {tooltip && (
        <InfoTooltip paperCss={{ maxWidth: '300px' }}>{tooltip}</InfoTooltip>
      )}
    </div>
  );
};

export const FleetColumnHeader = <TData extends MRT_RowData>({
  header,
  headerText,
  infoTooltip,
}: FleetColumnHeaderProps<TData>) => {
  const { column } = header;
  const { columnDef } = column;

  // Resolve content: Prop > ColumnDef > ID
  const resolvedText = headerText ?? (columnDef.header as string) ?? column.id;
  const resolvedTooltip = infoTooltip ?? columnDef.meta?.infoTooltip;

  return (
    <FleetColumnHeaderContent text={resolvedText} tooltip={resolvedTooltip} />
  );
};
