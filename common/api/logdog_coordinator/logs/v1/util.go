// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logdog

// GetMessageProject implements ProjectBoundMessage.
func (r *GetRequest) GetMessageProject() string { return r.Project }

// GetMessageProject implements ProjectBoundMessage.
func (r *TailRequest) GetMessageProject() string { return r.Project }

// GetMessageProject implements ProjectBoundMessage.
func (r *QueryRequest) GetMessageProject() string { return r.Project }
