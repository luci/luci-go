package git

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MarshalJSON implements json.Marshaler.
func (c *Commit_TreeDiff_ChangeType) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// MarshalJSON implements json.Unmarshaler.
func (c *Commit_TreeDiff_ChangeType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	change, ok := Commit_TreeDiff_ChangeType_value[strings.ToUpper(s)]
	if !ok {
		return fmt.Errorf("unexpected change type %q", s)
	}

	*c = Commit_TreeDiff_ChangeType(change)
	return nil
}
