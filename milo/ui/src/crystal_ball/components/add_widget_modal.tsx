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
  Article as ArticleIcon,
  ShowChart as ShowChartIcon,
  TableChart as TableChartIcon,
} from '@mui/icons-material';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Typography,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  ListItemIcon,
} from '@mui/material';
import { useState } from 'react';

import { WidgetType } from '@/crystal_ball/types';

export interface AddWidgetModalProps {
  open: boolean;
  onClose: () => void;
  /**
   * Callback fired to add a new widget. Returns the selected type identifier.
   */
  onAdd: (widgetType: WidgetType) => void;
}

/**
 * A modal to select a widget type to add to the dashboard.
 */
export function AddWidgetModal({ open, onClose, onAdd }: AddWidgetModalProps) {
  const [selectedType, setSelectedType] = useState<WidgetType | null>(null);

  const handleAdd = () => {
    if (selectedType) {
      onAdd(selectedType);
      setSelectedType(null);
    }
  };

  const handleClose = () => {
    setSelectedType(null);
    onClose();
  };

  const widgetOptions = [
    {
      type: WidgetType.MARKDOWN,
      icon: ArticleIcon,
      primary: 'Markdown Widget',
      secondary: 'Add text, links, and simple formatting',
    },
    {
      type: WidgetType.CHART_MULTI_METRIC,
      icon: ShowChartIcon,
      primary: 'Multi Metric Chart',
      secondary: 'Display time series data for multiple metrics',
    },
    {
      type: WidgetType.CHART_BREAKDOWN_TABLE,
      icon: TableChartIcon,
      primary: 'Breakdown Table',
      secondary: 'Display breakdown data in a table view',
    },
  ];

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Add Widget</DialogTitle>
      <DialogContent dividers>
        <Typography variant="body2" color="text.secondary" gutterBottom>
          Select a widget type to add to your dashboard.
        </Typography>
        <List sx={{ width: '100%', bgcolor: 'background.paper' }}>
          {widgetOptions.map((option) => {
            const isSelected = selectedType === option.type;
            const IconComponent = option.icon;
            return (
              <ListItem key={option.type} disablePadding sx={{ mb: 1 }}>
                <ListItemButton
                  selected={isSelected}
                  onClick={() => setSelectedType(option.type)}
                  sx={{
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: isSelected ? 'primary.main' : 'divider',
                    '&:hover': {
                      borderColor: 'primary.light',
                    },
                  }}
                >
                  <ListItemIcon>
                    <IconComponent color={isSelected ? 'primary' : 'action'} />
                  </ListItemIcon>
                  <ListItemText
                    primary={option.primary}
                    secondary={option.secondary}
                  />
                </ListItemButton>
              </ListItem>
            );
          })}
        </List>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button
          onClick={handleAdd}
          variant="contained"
          disabled={!selectedType}
        >
          Add
        </Button>
      </DialogActions>
    </Dialog>
  );
}
