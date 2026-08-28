# Fleet Console E2E Browser Recipes

This folder contains Markdown-based end-to-end user flow recipes for Fleet Console UI pages.

> [!NOTE]
> **Recipe Scope**: Recipes in this folder are intended **only for well-established user journeys that will be run multiple times for automated regression testing**. One-off manual checks or exploratory edge cases should be tested directly via the Chrome DevTools MCP tools rather than committed as recipes.

## Recipe Format
Each recipe is a simple Markdown document structured as:
1. **Goal & Target URL**: What user journey is being tested and the starting page route.
2. **Prerequisites**: Services required (e.g. backend/DB only if changes were made to them not in dev).
3. **Action Steps with Embedded JSON**: Command action sequences embedded in `json` code blocks.
4. **Visual Verification Checklist**: Concrete criteria for an engineer or AI agent to verify when inspecting screenshot outputs with `view_file`.
5. **Flexible Selectors**: Selectors combine stable `data-testid` attributes with semantic text/ARIA fallbacks so recipes remain resilient if styling changes.

## Available Recipes
- [priority_scoring_rules.md](./priority_scoring_rules.md): Tests viewing, editing, creating, and validating priority scoring rules in the ChromeOS Repairs admin panel against live backend and database.

## Running Recipes
You can run any recipe block using `gbrowser batch` connected to Chrome debug port `9222`.
