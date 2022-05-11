// Copyright 2022 The LUCI Authors.
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

class ChangeLogContent {
  constructor(element, target, revision) {
    // Root element for change log table.
    this.element = document.getElementById(element);
    // If set, limits change log queries to given target.
    this.target = target;
    // If set, limits change log queries to specific revision only.
    this.revision = revision;
    // Page size.
    this.pageSize = 15;
    // Page token for the current page.
    this.pageToken = null;
    // Page token for the next page.
    this.nextPageToken = null;
    // Stack of page tokens for previous pages.
    this.prevPageTokens = [];
    // True is UI is locked (when doing AJAX).
    this.locked = false;

    // Attach events to pager buttons.
    var pager = document.getElementById('change-log-pager');
    var prev = pager.querySelector('#prev');
    var next = pager.querySelector('#next');
    prev.addEventListener('click', () => {
      this.prevClicked();
    })
    next.addEventListener('click', () => {
      this.nextClicked();
    })
  }

  // Refetches the current page.
  refresh() {
    var that = this;
    return this.refetchChangeLogs(this.pageToken, (response) => {
      that.setChangeLogList(response.changes);
      that.nextPageToken = response.nextPageToken;
    })
  }

  // Called when '<- Newer' button is clicked.
  prevClicked() {
    if (this.prevPageTokens.length <= 0 || this.locked) {
      return;
    }
    var prev = this.prevPageTokens[this.prevPageTokens.length - 1];
    var that = this;
    return this.refetchChangeLogs(prev, (response) => {
      that.setChangeLogList(response.changes);
      that.prevPageTokens.pop();
      that.pageToken = prev;
      that.nextPageToken = response.nextPageToken;
    })
  }

  // Called when 'Older ->' button is clicked.
  nextClicked() {
    if (!this.nextPageToken || this.locked) {
      return;
    }
    var next = this.nextPageToken;
    var that = this;
    return this.refetchChangeLogs(next, (response) => {
      that.setChangeLogList(response.changes);
      that.prevPageTokens.push(that.pageToken);
      that.pageToken = next;
      that.nextPageToken = response.nextPageToken;
    })
  }

  // Disables or enables pager buttons based on 'prevCursor' and 'nextCursor'.
  updatePagerButtons() {
    var pager = document.querySelector('#change-log-pager');
    var prev = pager.querySelector('#prev');
    var next = pager.querySelector('#next');

    var hasPrev = this.prevPageTokens.length > 0;
    prev.classList.toggle('disabled', !hasPrev);
    next.classList.toggle('disabled', !this.nextPageToken);
    pager.style.display = (hasPrev || this.nextPageToken) ? 'block' : 'none';
  }

  // Loads list of change logs from a server.
  // Updates change log list UI. Returns deferred.
  refetchChangeLogs(pageToken, callback) {
    var that = this;
    var defer = api.changeLogs(this.target, this.revision, this.pageSize, pageToken);
    this.lockUI();
    defer
      .then((response) => {
        callback(response);
        that.unlockUI();
        this.updatePagerButtons();
      })
      .catch((err) => {
        console.log(err);
        that.unlockUI();
        this.updatePagerButtons();
      });
    return defer;
  }

  setChangeLogList(logs) {
    const parseTarget = (t) => {
      var kind = t.split('$', 1)[0];
      var name = t.substring(kind.length + 1);

      // Recognize some known targets.
      var title = name;
      var targetURL = null;
      switch (kind) {
        case 'AuthGroup':
          targetURL = '/groups/' + name;
          break;
        case 'AuthIPWhitelist':
          targetURL = '/ip_allowlists';
          break;
        case 'AuthIPWhitelistAssignments':
          title = 'IP allowlist assignment';
          targetURL = '/ip_allowlists';
          break;
        case 'AuthGlobalConfig':
          title = 'Global config';
          targetURL = '/oauth_config';
          break;
        case 'AuthRealmsGlobals':
          title = 'Realms config';
          break;
      }

      return {
        kind: kind,
        name: name,
        title: title,
        changeLogTargetURL: common.getChangeLogTargetURL(kind, name),
        targetURL: targetURL
      };
    }

    const addElement = (log) => {
      if ('content' in document.createElement('template')) {
        var template = document.querySelector('#change-log-row-template')

        // Clone and grab elements to modify.
        var clone = template.content.cloneNode(true);
        var rev = clone.querySelector('td.change-log-rev a');
        var type = clone.querySelector('td.change-log-type');
        var when = clone.querySelector('td.change-log-when');
        var who = clone.querySelector('td.change-log-who');
        var target = clone.querySelector('td.change-log-target a');

        // Modify contents and append to parent.
        var t = parseTarget(log.target);
        rev.href = common.getChangeLogRevisionURL(log.authDbRev);
        rev.textContent = 'r' + log.authDbRev;
        type.textContent = log.changeType;
        when.textContent = common.utcTimestampToString(log.when);
        who.textContent = common.stripPrefix('user', log.who);
        target.href = t.changeLogTargetURL;
        target.textContent = t.title;

        this.element.appendChild(clone);
      } else {
        // TODO: Find another way to add changeLogContent because the
        // HTML template element is not supported.
        console.error('Unable to load HTML template element, not supported.');
      }
    }

    this.element.replaceChildren();
    logs.map((log) => {
      addElement(log);
    });
  }

  // Locks UI actions before AJAX.
  lockUI() {
    this.locked = true;
  }

  // Unlocks UI actions after AJAX.
  unlockUI() {
    this.locked = false;
  }
}

window.onload = () => {
  var changeLogContent = new ChangeLogContent('change-log-content')
  changeLogContent.refresh();
}