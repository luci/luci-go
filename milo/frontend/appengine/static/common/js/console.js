$(function () {
  'use strict';
  function resizeHeight() {
    // Find the row height using the first console-cell-container of
    // each console-leaf-category.
    console.log("Resizing");
    var rowHeight = 200;  // Minimum height.
    $(".console-leaf-category").each(function() {
      var thisContainer = $(".console-build-column-stacked .console-cell-container-inner").first();
      var thisHeight = thisContainer.height();
      if (thisHeight > rowHeight) rowHeight = thisHeight;
    });
    // Now set all cells to be the same height, as well as commit descriptions;
    $(".console-cell-container").height(rowHeight);
    $(".console-commit-item").height(rowHeight - 1);  // 1px for the border.
  }

  $(".control-expand").click(function(e) {
    e.preventDefault();
    $("#console").removeClass("collapsed");
    $("#console").addClass("expanded");
    let width = 0;
    $("#console > div").each(function() {
      width += $(this).width();
    });

    // Figure out the number of rows.
    var numRows = 0;
    $(".console-commit-item").each(function() {
      numRows++;
    });

    // Stack the console.
    $(".console-leaf-category").each(function() {
      // Contains a matrix of cells.
      // Used to contain the actual bubbles.
      const rows = [];
      for (var i = 0; i < numRows; i++) {
        rows.push([]);
      }

      // Use the first column to find out the number of console spaces.
      const spaces = $(this).find(".console-builder-column").first().find(".console-space").length;

      // Gather the bubbles, populate the matrix.
      $(this).find(".console-builder-column").each(function() {
        $(this).find(".console-cell-container").each(function(i) {
          rows[i].push($(this).find("a")[0]);
        });
        $(this).hide();
      });

      // Create a new column for the leaf category.
      const newBuilderColumn = $(document.createElement("div"));
      newBuilderColumn.addClass("console-builder-column");
      newBuilderColumn.addClass("stacked");
      for (var i = 0; i < spaces; i++) {
        // Pad the header with spaces.
        newBuilderColumn.append("<div class=\"console-space\"></div>");
      }
      const newColumn = $(document.createElement("div"));
      newColumn.addClass("console-build-column-stacked")
      newBuilderColumn.append(newColumn);

      // Populate each row.
      for (const i in rows) {
        const row = rows[i];
        const newContainer = $(document.createElement("div"));
        newContainer.addClass("console-cell-container");
        newColumn.append(newContainer);
        // We need an inner container to calculate height.
        const newInnerContainer = $(document.createElement("div"))
        newInnerContainer.addClass("console-cell-container-inner");
        newContainer.append(newInnerContainer);

        for (const j in row) {
          newInnerContainer.append($(row[j]).clone());
        }
      }

      // Stick the new column in the leaf.
      $(this).append(newBuilderColumn);
    });

    resizeHeight();
    $(window).resize(resizeHeight);

    $(".control-collapse").show();
    $(".control-expand").hide();
    $(".console-builder-item").hide();  // Hide the builder boxes.
    Cookies.set('expand', 1);
  });

  $(".control-collapse").click(function(e) {
    e.preventDefault();
    $("#console").addClass("collapsed");
    $("#console").removeClass("expanded");
    $(".control-expand").show();
    $(".control-collapse").hide();
    $(".stacked").remove();
    $(".console-builder-item").show();    // Show the builder boxes.
    $(".console-builder-column").show();  // Show the collapsed console.
    $(".console-commit-item").each(function() {
      $(this).css("min-height", "");
    });
    $(".console-cell-container").height("auto");
    $(".console-commit-item").height("auto");
    Cookies.remove('expand');
  });
  if (Cookies.get('expand')) {
    $(".control-expand").first().click();
  }
});
