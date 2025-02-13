A text box based autocomplete component.

## Notable differences when compared to `<Autocomplete />` from MUI:

### No split between selected options and input.
MUI treats selected options and input separately. Autocomplete can only suggest
edit to the new input. It's difficult/impossible to edit selected options unless
they are removed first.

This component combines selected options and input into a single value state.
This allows us to support editing any part of the value.

### Specify how options are generated/applied via functions.
MUI takes a list of options. It filters the options by their label substring.
It applies selected options using the predefined operation (replace or append).

This component uses functions to govern when options should be updated and how
options should be applied. This gives us the flexibility to generate and apply
options depending on the text cursor position.
