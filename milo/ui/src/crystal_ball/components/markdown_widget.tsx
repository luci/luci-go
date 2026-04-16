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

import { Edit } from '@mui/icons-material';
import {
  Box,
  Button,
  IconButton,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { PerfWidget } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

export interface MarkdownWidgetProps {
  /**
   * The actual widget data from the dashboard state.
   */
  widget: PerfWidget;
  /**
   * Callback fired when the markdown content is updated.
   */
  onUpdate: (updatedWidget: PerfWidget) => void;
}

/**
 * A widget that renders markdown content and provides an inline editing interface.
 */
export function MarkdownWidget({ widget, onUpdate }: MarkdownWidgetProps) {
  const [markdown, setMarkdown] = useState('');
  const [isEditing, setIsEditing] = useState(false);

  const handleSave = () => {
    onUpdate({
      ...widget,
      markdown: {
        ...widget.markdown,
        content: markdown,
      },
    });
    setIsEditing(false);
  };

  const handleCancel = () => {
    setIsEditing(false);
  };

  const renderedHtml = useMemo(
    () => renderMarkdown(widget.markdown?.content || ''),
    [widget.markdown?.content],
  );

  return (
    <Box sx={{ px: 0, py: 0 }}>
      {isEditing ? (
        <Box>
          <TextField
            multiline
            rows={4}
            fullWidth
            value={markdown}
            onChange={(e) => setMarkdown(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                handleCancel();
              }
            }}
            placeholder="Enter markdown content..."
            variant="outlined"
          />
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'flex-end',
              gap: 1,
              mt: 2,
            }}
          >
            <Button onClick={handleCancel} color="inherit">
              Cancel
            </Button>
            <Button
              onClick={handleSave}
              variant="contained"
              disableElevation
              color="primary"
            >
              Apply
            </Button>
          </Box>
        </Box>
      ) : (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'flex-end',
            gap: 1,
            '&:hover .markdown-edit-btn': { opacity: 1 },
          }}
        >
          <Box sx={{ flexGrow: 1, minWidth: 0, '& > *:last-child': { mb: 0 } }}>
            {widget.markdown?.content?.trim() ? (
              <SanitizedHtml html={renderedHtml} />
            ) : (
              <Typography sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                No content. Click to edit.
              </Typography>
            )}
          </Box>
          <Box sx={{ display: 'flex', flexShrink: 0 }}>
            <Tooltip title={COMMON_MESSAGES.EDIT}>
              <IconButton
                size="small"
                onClick={() => {
                  setMarkdown(widget.markdown?.content || '');
                  setIsEditing(true);
                }}
                className="markdown-edit-btn"
                sx={{
                  opacity: 0,
                  transition: 'opacity 0.2s',
                  width: 24,
                  height: 24,
                }}
              >
                <Edit fontSize="small" sx={{ color: 'text.secondary' }} />
              </IconButton>
            </Tooltip>
          </Box>
        </Box>
      )}
    </Box>
  );
}
