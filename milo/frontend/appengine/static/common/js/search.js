// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// A library for doing filtering with lists.


(function(window) {
  'use strict';

  /**
   * Given a root node, hide all elements that don't match the query.  The
   * expected structure is:
   * <div>
   *   <div class="filterable" data="text">
   *     <li class="filterable" data="text"></li>
   *   </div>
   * </div>
   *
   * This searches the data tags under a root node.  For all elements that
   * match, all of its ancestors and descendants are marked visible.
   *
   * All data is expected to be in lower case.  This allows case-insensitive
   * search.
   *
   */
  function filter(root, query) {
    // No query? Make sure everything is visible, and then we're done here.
    if (!query) {
      root.find(".filterable").each(function() {
        $(this).toggle(true);
      });
      return;
    }

    query = query.toLowerCase();

    // Create a jQuery selector for all words that are substrings.
    const visible_items = new Set();
    var selector = "*";
    for (const part of query.split(" ")) {
      if (part) {
        selector += "[data*='" + $.escapeSelector(part) + "']";
      }
    }

    // Make sure the element, all ancestors, and all descendants are visible.
    root.find(selector).each(function() {
      visible_items.add(this);
      $(this).parents(".filterable").each(function() {
        visible_items.add(this);
      });
      $(this).find(".filterable").each(function() {
        visible_items.add(this);
      });
    });

    root.find(".filterable").each(function() {
      // Hide the element if it is not in the visible items group.
      $(this).toggle(!!visible_items.has(this));
    });
  }

  /* Return the given url parameter from the current url */
  function getUrlParameter(param) {
    const pageURL = decodeURIComponent(window.location.search.substring(1));
    const components = pageURL.split('&');

    for (const component of components) {
      let [key, value] = component.split('=', 2);
      if (key === param) {
          return value === undefined ? "" : value;
      }
    }
  }

  // Export all as module level functions.
  Object.assign(window.milo = window.milo || {}, {
    filter, getUrlParameter
  });

}(window));

