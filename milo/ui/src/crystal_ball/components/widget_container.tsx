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
  ArrowDownward as ArrowDownwardIcon,
  ArrowUpward as ArrowUpwardIcon,
  Delete as DeleteIcon,
  Edit as EditIcon,
  LibraryAdd as LibraryAddIcon,
} from '@mui/icons-material';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { ReactNode, useRef, useState } from 'react';
import {
  ImperativePanelHandle,
  Panel,
  PanelGroup,
  PanelResizeHandle,
} from 'react-resizable-panels';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { WidgetPortalContext } from '@/crystal_ball/context';

/**
 * Props for the WidgetContainer component.
 */
export interface WidgetContainerProps {
  title: string;
  children: ReactNode;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  onDelete?: () => void;
  onDuplicate?: () => void;
  /**
   * Whether the move up action should be disabled.
   */
  disableMoveUp?: boolean;
  /**
   * Whether the move down action should be disabled.
   */
  disableMoveDown?: boolean;
  onTitleChange?: (newTitle: string) => void;
  /**
   * Whether to remove the default padding from the container's content area.
   */
  disablePadding?: boolean;
}

/**
 * A container component for dashboard widgets.
 * Provides a consistent card layout with a header, title, and standard actions
 * (move up, move down, delete).
 */
export function WidgetContainer({
  title,
  children,
  onMoveUp,
  onMoveDown,
  onDelete,
  onDuplicate,
  disableMoveUp = false,
  disableMoveDown = false,
  onTitleChange,
  disablePadding = false,
}: WidgetContainerProps) {
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [editedTitle, setEditedTitle] = useState('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  const handleTitleSubmit = () => {
    setIsEditingTitle(false);
    if (onTitleChange) {
      onTitleChange(editedTitle);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleTitleSubmit();
    if (e.key === 'Escape') {
      setIsEditingTitle(false);
      setEditedTitle(title);
    }
  };

  const [isOpen, setOpen] = useState(false);
  const [portalTarget, setPortalTarget] = useState<HTMLDivElement | null>(null);
  const [isFolded, setIsFolded] = useState(false);
  const sidePanelRef = useRef<ImperativePanelHandle>(null);
  const [previousSize, setPreviousSize] = useState(30);
  const fold = () => sidePanelRef.current?.resize(5);
  const expand = () => sidePanelRef.current?.resize(previousSize);

  return (
    <WidgetPortalContext.Provider
      value={{ target: portalTarget, setOpen, isOpen, isFolded, fold, expand }}
    >
      <Box
        sx={{
          display: 'flex',
          position: 'relative',
          gap: 1,
          marginBottom: '16px',
        }}
      >
        <PanelGroup direction="horizontal" style={{ flex: 1 }}>
          <Panel defaultSize={isOpen && !isFolded ? 70 : 100} minSize={30}>
            <Card variant="outlined" sx={{ width: '100%', minWidth: 0 }}>
              <CardHeader
                sx={{
                  px: 2,
                  py: 1.5,
                  '& .MuiCardHeader-action': { alignSelf: 'center', m: 0 },
                  '&:hover .title-edit-btn': { opacity: 1 },
                }}
                title={
                  isEditingTitle ? (
                    <TextField
                      value={editedTitle}
                      onChange={(e) => setEditedTitle(e.target.value)}
                      onBlur={handleTitleSubmit}
                      onKeyDown={handleKeyDown}
                      size="small"
                      variant="standard"
                      sx={{
                        '& .MuiInputBase-input': {
                          fontSize: (theme) => theme.typography.body1.fontSize,
                          fontWeight: (theme) =>
                            theme.typography.fontWeightMedium,
                        },
                      }}
                    />
                  ) : (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography
                        variant="body1"
                        sx={{
                          fontWeight: (theme) =>
                            theme.typography.fontWeightMedium,
                        }}
                      >
                        {title}
                      </Typography>
                      {onTitleChange && (
                        <Tooltip title={COMMON_MESSAGES.EDIT_TITLE}>
                          <IconButton
                            size="small"
                            onClick={() => {
                              setEditedTitle(title);
                              setIsEditingTitle(true);
                            }}
                            aria-label={`Edit title ${title}`}
                            className="title-edit-btn"
                            sx={{
                              width: 24,
                              height: 24,
                              opacity: 0,
                              transition: 'opacity 0.2s',
                            }}
                          >
                            <EditIcon
                              fontSize="small"
                              sx={{ color: 'text.secondary' }}
                            />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Box>
                  )
                }
                action={
                  <Box sx={{ display: 'flex', gap: 0.5 }}>
                    {onMoveUp && (
                      <Tooltip title={COMMON_MESSAGES.MOVE_UP}>
                        <span>
                          <IconButton
                            size="small"
                            onClick={onMoveUp}
                            disabled={disableMoveUp}
                            aria-label={`Move ${title} up`}
                          >
                            <ArrowUpwardIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    )}
                    {onMoveDown && (
                      <Tooltip title={COMMON_MESSAGES.MOVE_DOWN}>
                        <span>
                          <IconButton
                            size="small"
                            onClick={onMoveDown}
                            disabled={disableMoveDown}
                            aria-label={`Move ${title} down`}
                          >
                            <ArrowDownwardIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    )}
                    {onDuplicate && (
                      <Tooltip title={COMMON_MESSAGES.DUPLICATE}>
                        <IconButton
                          size="small"
                          onClick={onDuplicate}
                          aria-label={`Duplicate ${title}`}
                        >
                          <LibraryAddIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    )}
                    {onDelete && (
                      <Tooltip title={COMMON_MESSAGES.DELETE}>
                        <IconButton
                          size="small"
                          onClick={() => setDeleteDialogOpen(true)}
                          aria-label={`Delete ${title}`}
                          sx={{ color: 'text.secondary' }}
                        >
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    )}
                  </Box>
                }
              />
              <Divider />
              <CardContent
                sx={
                  disablePadding
                    ? { p: 0, '&:last-child': { pb: 0 } }
                    : { p: 2, '&:last-child': { pb: 2 } }
                }
              >
                <Box
                  sx={{
                    display: 'grid',
                    gridTemplateColumns: 'minmax(0, 1fr)',
                  }}
                >
                  {children}
                </Box>
              </CardContent>
              <Dialog
                open={deleteDialogOpen}
                onClose={() => setDeleteDialogOpen(false)}
              >
                <DialogTitle>Delete Widget</DialogTitle>
                <DialogContent>
                  <Typography>
                    Are you sure you want to delete this widget?
                  </Typography>
                </DialogContent>
                <DialogActions sx={{ p: 2, pt: 0 }}>
                  <Button
                    onClick={() => setDeleteDialogOpen(false)}
                    color="inherit"
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={() => {
                      setDeleteDialogOpen(false);
                      onDelete?.();
                    }}
                    color="error"
                    variant="contained"
                    disableElevation
                  >
                    Delete
                  </Button>
                </DialogActions>
              </Dialog>
            </Card>
          </Panel>
          {isOpen && (
            <>
              <PanelResizeHandle>
                <Box
                  sx={{
                    width: '8px',
                    height: '100%',
                    cursor: 'col-resize',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    bgcolor: 'action.hover',
                    '&:hover': { bgcolor: 'action.selected' },
                  }}
                >
                  <Box
                    sx={{ width: '2px', height: '24px', bgcolor: 'divider' }}
                  />
                </Box>
              </PanelResizeHandle>
              <Panel
                ref={sidePanelRef}
                defaultSize={30}
                minSize={5}
                collapsible={false}
                onResize={(size) => {
                  setIsFolded(size <= 5);
                  if (size > 5) {
                    setPreviousSize(size);
                  }
                }}
                style={{ position: 'relative' }}
              >
                <Box
                  ref={setPortalTarget}
                  sx={{
                    position: 'absolute',
                    top: 0,
                    bottom: 0,
                    left: 0,
                    right: 0,
                    overflow: 'hidden',
                  }}
                />
              </Panel>
            </>
          )}
        </PanelGroup>
      </Box>
    </WidgetPortalContext.Provider>
  );
}
