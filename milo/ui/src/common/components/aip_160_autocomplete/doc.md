# AIP 160 Autocomplete

An input component that can render autocomplete suggestions to an
[AIP-160 filter](http://go/aip/160) according to the given schema.

Try it [here](https://ci.chromium.org/ui/tests/p/chromium/clusters).

## Sample schema
```tsx
// Acquire the dependencies required to construct the schema.
const client = useMyPrpcClient();

const schema: FieldDef = {
  staticFields: {
    my_field_with_no_value_suggestion: {},
    my_field_with_sync_suggestion: {
      // Partial is the text value of the value token near the text cursor.
      getValues: (partial) => {
        // Filter/compute the list of value suggestions for the given user input.
        return ['true', 'false']
          .filter((text) => text.includes(partial) && text !== partial)
          .map((text) => ({ text }));
      },
    },
    my_field_with_async_value_suggestion: {
      // Compute a ReactQuery UseQueryOptions that can return a list of value
      // suggestions for the given user input.
      fetchValues: (partial) => ({
        ...client.MyPrpcMethod.query({
          // If needed, you can include the user input in the query params.
          queryParam1: partial,
          // Specify other query params.
          queryParam2: "other-query-params",
        }),
        select: (res) => {
          // Usually, the response from the RPC does not match the required type
          // signature. You can transform it by specifying the `select` option.
          // See react-query documentation for details.
        }
      }),
    },
    my_parent_field: {
      // You can also specify nested fields.
      staticFields: {
        my_child_field1: {},
        my_child_field1: {},
        tags: {
          // If the fields can not be enumerated, you can also specify dynamic
          // fields. This is often useful for building autocomplete for
          // key-value pairs.
          dynamicFields: {
            getKeys: (partial) => {
              return [
                {
                  text: partial,
                }
              ];
            }
            getValues: (key, partial) => {
              return [
                {
                  text: partial,
                }
              ];
            }
          }
        },
      },
    },
  },
};
```

## Design
There are 4 pieces that keep this together.

```txt
                                        ┌───────────────────┐
                                        │   FieldsSchema    │
                                        ├───────────────────┤
                                        │ Defines fields,   │
                                        │ values, and how to│
                                        │ fetch/compute     │
                                        │ them              │
                                        └───────────────────┘
                                                  │
                                                  │ Guides suggestions
                                                  ▼
┌───────────────────┐                   ┌───────────────────┐                        ┌───────────────────┐
│       Lexer       │                   │ Suggestion Engine │                        │ TextAutocomplete  │
├───────────────────┤                   ├───────────────────┤                        ├───────────────────┤
│ Tokenizes user    │  Provides tokens  │ Uses schema &     │  Provides suggestions  │ Renders & handles │
│ input.            │──────────────────►│ token context to  │───────────────────────►│ suggestions.      │
│                   │                   │ generate          │                        │ Applies selected  │
│                   │                   │ suggestions.      │                        │ suggestion to     │
│                   │                   │                   │                        │ user input.       │
└───────────────────┘                   └───────────────────┘                        └───────────────────┘
          ▲                                                                                    │
          │                                Provides user input                                 │
          └────────────────────────────────────────────────────────────────────────────────────┘
```

### Schema (`FieldsSchema`)
Schema defines
 * the available fields and values and their relationship (e.g. nested field).
 * how to compute the available fields and values.
 * how to display the fields and values.

The schema is a JS object that contains closures. See the type definition for
details.

#### Alternatives considered
##### Serializable (e.g. text-based) schema
Make the schema serializable allow us to achieve the followings:
 * Make the server the single source of truth of an AIP-160 filter schema.
   This is fitting because ultimately, the server decides what AIP-160 filter
   is accepted and how the filter behaves. We don't need to coordinate UI
   and server releases when the schema changes.
 * Potentially generate the schema from database DDL.

However, there are many challenges to this.
 * It might be costly or impossible to enumerate all the possible values for a
   field. As such, we need a way to specify how to compute/retrieve a list of
   possible values.
 * When we compute/retrieve a list of possible values, often the function/
   endpoint requires some contextual information, including but not limited to
    * the selected project
    * the authentication token
    * partial user input
    * the address to fetch the values from
    * the transformation needed
    * the way to put all the required contexts into a request/function.

Those challenges can be easily solved using closures. But using closure also
means the schema is not serializable, therefore the schema must be hard coded on
the UI side.

Given that
 * We only have a single UI (LUCI UI).
 * LUCI UI is deployed automatically.
 * Slightly out-of-sync schema only breaks the autocomplete not the actual
   filter.

Hardcoding the schema should be acceptable.

##### Make `fetchValues` return a hook rather than a ReactQuery option.
Making `fetchValues` return a hook has the following benefits:
1. It's easier to compose multiple queries as we can chain multiple
   `useQuery` hooks inside the hook.
2. We can conditionally require different (React) context for different
  `fetchValues` on different fields. At the moment, all the dependencies
  required to construct the `fieldSchema` needs to be instantiated upfront when
  constructing the schema.

However, the implementation is impossible (without horrible hacks) with
vanilla React state management tools because React requires the order of hooks
to remain unchanged once a component is initialized.

We might be able to achieve the benefits above if we opt to use a more flexible
state management framework (e.g. MobX). But given that vast majority of the
time, we should be able to get away with a single query, introducing a new
framework is likely not worth it.

Note that we can still chain multiple queries together in
`QueryOptions.queryFn`. They just need to share a single cache slot by default.
If we are desperate, we can always populate the cache manually using
`client.setQueryData`.

##### Make `fetchValues` return a promise.
This is almost the same as making it return a ReactQuery option, as ReactQuery
option contains a function to construct a promise anyway. But we lose the
benefits ReactQuery gives us (e.g. specifying the cache duration, cache key,
etc).

### Lexer
Lexer converts the user input into tokens. It allows us to write the rules in
the suggestion engine based on the currently selected token rather than just
plain input.

#### Alternatives considered
##### EBNF parser
Instead of building our own lexer, it might be tempting to use one of the third
party EBNF parser library. Then we can use [the provided EBNF grammar](https://google.aip.dev/assets/misc/ebnf-filtering.txt)
for the AIP-160 filter to parse user input into an AST.

This doesn't work due to the following problems:
 1. EBNF grammar only defines what is considered a valid string for the given
    grammar. It is not always suitable for parsing. The same string could be
    be interpreted into different ASTs using the same grammar when the rules are
    resolved in different orders.
 2. The user input may not be a valid AIP-160 filter. This is especially true if
    the user is still editing the filter (which is the exact time when
    autocomplete is needed). As such, the EBNF grammar needs to be expanded to
    tolerate invalid inputs. This causes problem 1 to be even more severe as
    each rule in the EBNF grammar now must expand to accept more cases.
    Therefore there will be more alternative interpretation of the same input
    string.

##### Share the lexer implementation with a backend AIP-160 parser
Frontend lexer has a different set of requirements compared to a backend lexer.

1. The frontend lexer needs to be a lot more error tolerant than a backend lexer
   because we need to (partially) understand the invalid/incomplete filter as
   the user is still editing it.
2. The frontend lexer needs to know more metadata (e.g. the start and end
   index of a token) because it needs to suggest edit to the filter string.
3. Due to the lack of parser to parse the tokens into an AST (which is in turn
   due to that the filter is likely invalid when user is still editing them),
   the frontend lexer prefers to produce larger tokens so building a suggestion
   engine is easier. For example, the backend lexer implementation may prefer
   treating `my.nested.field` as 5 tokens (3 text tokens and 2 dot tokens). The
   full nested field can be retrieved at a higher level in the AST tree. On the
   other hand, the frontend lexer prefers treating `my.nested.field` as a single
   token because otherwise the suggestion engine needs to traverse multiple
   tokens to get the nested field. This can lead to complicated suggestion rule
   specification.

### Suggestion Engine (`useSuggestions`).
The suggestion engine looks at the currently selected token, gather contexts by
looking at nearby tokens, and make suggestion based on the provided schema.

The suggestion engine is token based not AST based because the filter may be
invalid/incomplete (as the user is still editing it) therefore cannot be parsed
into an AST.

### TextAutocomplete
TextAutocomplete handles the actual autocomplete functionality, includes
suggestion rendering, selection, application, etc.

See the documentation for `@/generic_libs/text_autocomplete` for details.

## Limitations
### No support for fetching a list of keys/values for dynamic fields from an RPC.
Currently, all fields must be statically defined (specified in `staticFields`)
or can be computed synchronously (specified in `dynamicFields`). If the list of
available fields need to be queried from an RPC (e.g. `variant.[key]`), the
current schema design does not let you specify that.

We can support this feature using an approach similar to how we use
`staticFields.[field].fetchValues` to support querying values for a static field
from an RPC.

At the time of writing this CL, this is not useful due to the next limitation.

### Unable to incorporate the rest of the filter when computing suggestions.
When computing suggestions, the current design only look at the nearby tokens.
It's unable to incorporate the rest of filter.

For example, to query a list of possible keys and values for
`variant.[key] = [value]`, we can use the
`luci.analysis.v1.TestHistory.QueryVariants` RPC. However, that RPC requires a
`test_id`, which means we need to parse the `test_id` from the rest of the
filter.

It's difficult to support this feature while ensuring consistent user experience
due to the following reasons:
 * The filter may not be valid. Trying to extracting value from an invalid
   filter can be difficult or inconsistent. This is especially true when logical
   operations are applied to the sub-filter we need
   (e.g. `(NOT test_id:mytest AND test_id:myothertest)`).
 * User may not specify the required filters to begin with.
