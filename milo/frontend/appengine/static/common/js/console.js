// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Functions for rendering the Milo console.
// Requires: jquery
//

$(function () {
  'use strict';

  // Figure out the number of rows, which is equal to the number of commits.
  const numRows = $('.console-commit-item').length;

  /**
   * Resizes commit cells when the window resizes.
   *
   * In the expanded view, the width is set by the browser window size.
   * When the window sizes changes, it changes the height of all the cells
   * because the cells contains elements that flow left to right.
   * Because the commit description on the left size are disjoint from
   * the cells, the height of the commit descriptions need to be updated.
   */
  function resizeHeight() {
    if ($('#console').hasClass('collapsed')) {
      return;  // Don't do anything in collapsed mode.
    }
    // Find the row height using the first console-cell-container of
    // each console-leaf-category.
    var rowHeight = 200;  // Minimum height.
    // Find the max height of each of the leaf category rows.
    $('.console-leaf-category').each(function() {
      const thisContainer = $('.console-build-column-stacked .console-cell-container-inner').first();
      const thisHeight = thisContainer.height();
      if (thisHeight > rowHeight) rowHeight = thisHeight;
    });

    // Now set all cells to be the same height, as well as commit descriptions;
    $('.console-cell-container').height(rowHeight);
    $('.console-commit-item').each(function() {
      $(this).height(rowHeight - 1);  // 1px for the border.
      const desc = $(this).find('.console-commit-description').first();
      const text = desc.find('p').first();
      const isClipped = desc.height() < text.height();
      // Add a class if the commit description is clipped,
      // so that we can render a fadeout.
      $(this).toggleClass('bottom-hidden', isClipped);
    });

    // Also set the width of the horizontal cell separator.
    var width = 0;
    $('#console>.console-column').each(function() {width += $(this).width();});
    $('.console-commit-item-overlay').width(width);
  }

  /**
  /* Given a leaf category as a jQuery element, return the matrix of cells.
   *
   * @param {.console-leaf-category node} category A jQuery node containing
   * a leaf ctegory.
   *
   * @returns {object} A data object, containing 'rows' and 'spaces'.
   * rows is a matrix of cells, which are <a> nodes coorisponding to a commit
   * and builder.
   * spaces is the number of spaces needed to pad the top of the column.
   */
  function getLeafCategoryData(category) {
    // The data source.  Each column represents a single builder.
    const builders = category.find('.console-builder-column');

    // Use the first column to find out the number of console spaces.
    // This is used to pad empty space in the categories.
    const spaces = builders.first().find('.console-space').length;

    // Contains a matrix of cells.
    // Used to contain the actual bubbles.
    const rows = [];
    for (let i = 0; i < numRows; i++) {
      rows.push([]);
    }

    // Gather the bubbles, populate the matrix.
    builders.each(function() {
      $(this).find('.console-cell-container').each(function(i) {
        rows[i].push($(this).find('a').first());
      });
    });

    return {
      rows: rows,
      spaces: spaces
    };
  }

  /**
   * Builds a leaf column from a matrix of rows.
   *
   * @param {object} data An object containing rows and spaces.
   *
   * @return {node} A console-builder-column to be appened under the leaf category.
   */
  function newLeafColumn(data) {
    // Create a new column for the leaf category.
    const newBuilderColumn = $(document.createElement('div'));
    newBuilderColumn.addClass('console-builder-column');
    newBuilderColumn.addClass('stacked');
    for (let i = 0; i < data.spaces; i++) {
      // Pad the header with spaces.
      newBuilderColumn.append('<div class="console-space"></div>');
    }
    const newColumn = $(document.createElement('div'));
    newColumn.addClass('console-build-column-stacked')
    newBuilderColumn.append(newColumn);

    // Populate each row.
    for (const row of data.rows) {
      const newContainer = $(document.createElement('div'));
      newContainer.addClass('console-cell-container');
      newColumn.append(newContainer);
      // We need an inner container to calculate height.
      const newInnerContainer = $(document.createElement('div'))
      newInnerContainer.addClass('console-cell-container-inner');
      newContainer.append(newInnerContainer);

      for (const item of row) {
        newInnerContainer.append($(item).clone());
      }
    }
    return newBuilderColumn;
  }

  /**
   * Overrides the default expand/collapse state of the console in the cookie.
   *
   * @param {bool} overrideDefault whether or not the user wants the default state.
   * If default is expand, and the user requested expand, this should be false.
   * If default is expand, and the user requested collapse, this should be true.
   * If default is collapse, and the user requested expand, this should be true.
   * If default is collapse, and the user requested collapse, this should be false.
   */
  function setCookie(overrideDefault) {
    if (overrideDefault) {
      Cookies.set('non-default', 1, {path: ''})
    } else {
      Cookies.remove('non-default', {path: ''})
    }
  }

  // Collapsed Mode -> Expanded Mode.
  $('.control-expand').click(function(e) {
    e.preventDefault();
    // Switch the top level class so that the expanded view nodes render.
    // TODO(hinoka): Refactor CSS so that we only need the expanded class.
    $('#console').removeClass('collapsed');
    $('#console').addClass('expanded');

    // Stack the console.
    $('.console-leaf-category').each(function() {
      const category = $(this);
      const data = getLeafCategoryData(category);
      const newColumn = newLeafColumn(data);

      // Hide the original columns.
      category.find('.console-builder-column').hide();
      // Stick the new column in the leaf.
      category.append(newColumn);
    });

    resizeHeight();
    $(window).resize(resizeHeight);

    $('.console-builder-item').hide();  // Hide the builder boxes.

    // Default expand, requested expand - False
    // Default collapse, requested expand - True
    setCookie(!defaultExpand);
  });

  // Expanded Mode -> Collapsed Mode.
  $('.control-collapse').click(function(e) {
    e.preventDefault();
    $('#console').addClass('collapsed');
    $('#console').removeClass('expanded');

    $('.stacked').remove();  // Delete all of the expanded elements.
    $('.console-builder-item').show();    // Show the builder boxes.
    $('.console-builder-column').show();  // Show the collapsed console.
    $('.console-cell-container').height('auto');
    $('.console-commit-item').height('auto');

    // Default expand, requested collapse - True
    // Default expand, requested expand - False
    setCookie(defaultExpand);
  });

  // We click on expand if:
  // Default is expand, and we don't see a cookie
  // Default is collapse, and we do see a cookie
  // Essentially we want to XOR the cookie bit with the default bit.
  if (Cookies.get('non-default') ? !defaultExpand : defaultExpand) {
    $('.control-expand').first().click();
  }
});
