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

import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Divider,
  SxProps,
  Theme,
} from '@mui/material';
import { ReactNode } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';

export interface InventoryDataCardProps {
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  editable?: boolean;
  isEditing?: boolean;
  onEdit?: () => void;
  onConfirm?: () => void;
  onDiscard?: () => void;
  headerAction?: ReactNode;
  children?: ReactNode;
  loading?: boolean;
  emptyMessage?: string;
  sx?: SxProps<Theme>;
}

export const InventoryDataCard = ({
  title,
  subtitle,
  icon,
  editable = false,
  isEditing = false,
  onEdit,
  onConfirm,
  onDiscard,
  headerAction,
  children,
  loading = false,
  emptyMessage,
  sx,
}: InventoryDataCardProps) => {
  return (
    <Card
      variant="outlined"
      sx={{ height: '100%', display: 'flex', flexDirection: 'column', ...sx }}
    >
      <CardHeader
        avatar={icon}
        title={title}
        subheader={subtitle}
        titleTypographyProps={{ variant: 'h6', fontWeight: 'bold' }}
        subheaderTypographyProps={{
          variant: 'caption',
          color: 'text.secondary',
        }}
        action={
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {headerAction}
            {editable && (
              <>
                {isEditing ? (
                  <>
                    {onDiscard && (
                      <Button size="small" onClick={onDiscard} color="inherit">
                        Cancel
                      </Button>
                    )}
                    {onConfirm && (
                      <Button
                        size="small"
                        variant="contained"
                        color="primary"
                        onClick={onConfirm}
                      >
                        Confirm
                      </Button>
                    )}
                  </>
                ) : (
                  onEdit && (
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={onEdit}
                      aria-label={`edit ${title}`}
                    >
                      Edit
                    </Button>
                  )
                )}
              </>
            )}
          </Box>
        }
      />
      <Divider />
      <CardContent sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
        {loading ? (
          <CentralizedProgress />
        ) : emptyMessage ? (
          <Alert severity="info">{emptyMessage}</Alert>
        ) : (
          children
        )}
      </CardContent>
    </Card>
  );
};
