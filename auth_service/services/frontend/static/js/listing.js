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

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// pluralized returns a string for the correctly pluralized noun, e.g.
// "0 members", "1 nested group", "5 globs".
const pluralized = (count, noun) => {
  if (count === 1) {
    return `1 ${noun}`;
  }
  return `${count} ${noun}s`;
}

////////////////////////////////////////////////////////////////////////////////
// GroupListing is a component which displays a group's full listing of
// memberships.
class GroupListing {
  constructor(data) {
    const template = document.querySelector('#group-listing-template');
    const clone = template.content.cloneNode(true);

    const name = data.name;
    const members = data.members || [];
    const globs = data.globs || [];
    const nestedGroups = data.nested || [];
    const redacted = data.numRedacted || [];

    const title = clone.querySelector('#title');
    title.textContent = name;
    title.href = common.getGroupPageURL(name);

    const totals = clone.querySelector('#summary-totals');
    const memberCount = members.length + redacted;
    let summary = pluralized(memberCount, 'member') + ` \u2014 `;
    if (globs.length > 0) {
      summary += pluralized(globs.length, 'glob') + ` \u2014 `;
    }
    summary += pluralized(nestedGroups.length, 'nested group');
    totals.textContent = summary;

    const memberRowCount = memberCount + globs.length;
    if (memberRowCount > 0) {
      const membersTableBody = clone.querySelector('#members-table > tbody');

      membersTableBody.innerHTML = '';

      if (redacted > 0) {
        const redactedRow = new RedactedMembersTableRow(redacted);
        membersTableBody.appendChild(redactedRow.element);
      }
      globs.forEach((glob) => {
        const globRow = new GlobTableRow(glob);
        membersTableBody.appendChild(globRow.element);
      });
      members.forEach((member) => {
        const memberRow = new MemberTableRow(member);
        membersTableBody.appendChild(memberRow.element);
      });
    }

    if (nestedGroups.length > 0) {
      const nestedGroupsTableBody = clone.querySelector('#nested-groups-table > tbody');

      nestedGroupsTableBody.innerHTML = '';

      nestedGroups.forEach((nested) => {
        const nestedGroupRow = new NestedGroupTableRow(nested);
        nestedGroupsTableBody.appendChild(nestedGroupRow.element);
      });
    }

    this.element = clone;
  }

}


////////////////////////////////////////////////////////////////////////////////
// DescendantTableRow is a table row, intended to be nested into a GroupListing
// table (members, globs or nested groups).
class DescendantTableRow {
  constructor(displayText, targetURL) {
    const template = document.querySelector('#descendant-item-template');
    const clone = template.content.cloneNode(true);
    this.element = clone.querySelector('tr');
    const linkEl = this.element.querySelector('a');
    linkEl.textContent = displayText;

    if (targetURL) {
      linkEl.setAttribute('href', targetURL);
    }
  }
}


////////////////////////////////////////////////////////////////////////////////
// RedactedMembersTableRow is a table row to specify the number of redacted
// members.
class RedactedMembersTableRow extends DescendantTableRow {
  constructor(redacted) {
    super(pluralized(redacted, 'member') + ' redacted', '');
  }
}


////////////////////////////////////////////////////////////////////////////////
// MemberTableRow is a table row for a single member.
class MemberTableRow extends DescendantTableRow {
  constructor(member) {
    const email = common.stripPrefix('user', member);
    super(email, common.getLookupURL(email));

    this.element.classList.add('border-top');
  }
}


////////////////////////////////////////////////////////////////////////////////
// GlobTableRow is a table row for a single glob pattern.
class GlobTableRow extends MemberTableRow {
  constructor(member) {
    super(member);
    this.element.classList.add('table-warning');
  }
}


////////////////////////////////////////////////////////////////////////////////
// NestedGroupTableRow is a table row for a single nested group.
class NestedGroupTableRow extends DescendantTableRow {
  constructor(group) {
    super(group, common.getGroupListingURL(group));
  }
}


////////////////////////////////////////////////////////////////////////////////
// Address bar manipulation.

const getCurrentGroupInURL = () => {
  return common.getQueryParameter('group');
}

////////////////////////////////////////////////////////////////////////////////


window.onload = () => {
  const integratedUIAlert = new common.IntegratedUIAlert('#integrated-ui-alert-container');
  integratedUIAlert.setLink(common.INTEGRATED_UI_GROUPS_ROOT);
  const loadingBox = new common.LoadingBox('#loading-box-placeholder');
  const groupListingSection = new common.HidableElement('#group-listing', false);
  const errorBox = new common.ErrorBox('#api-error-placeholder');

  const expandGroup = (group) => {
    groupListingSection.hide();
    errorBox.clearError();

    if (!group) {
      errorBox.showError(
        'Listing group memberships failed',
        'Invalid URL \u2014 no group specified.');
      return;
    }

    loadingBox.setLoadStatus(true);
    api.groupExpand(group)
      .then((response) => {
        groupListingSection.element.innerHTML = '';
        const groupListing = new GroupListing(response);
        groupListingSection.element.appendChild(groupListing.element);
        groupListingSection.show();
      })
      .catch((err) => {
        errorBox.showError('Listing group memberships failed', err.error);
      })
      .finally(() => {
        loadingBox.setLoadStatus(false);
      });
  };

  const group = getCurrentGroupInURL();
  integratedUIAlert.setLink(
    common.INTEGRATED_UI_GROUPS_ROOT + "/" + group + "?tab=descendants"
  )
  expandGroup(group);
}
