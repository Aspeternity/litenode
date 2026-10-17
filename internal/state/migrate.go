package state

import (
	"encoding/json"
	"fmt"
)

// Migration upgrades a decoded state document from version N to N+1 in
// place. It must not set schema_version; migrate does that.
type Migration func(doc map[string]any) error

// migrations maps a version N to the migration from N to N+1.
var migrations = map[int]Migration{}

func migrate(doc map[string]any, from, to int, table map[int]Migration) error {
	for v := from; v < to; v++ {
		m, ok := table[v]
		if !ok {
			return fmt.Errorf("no migration from schema version %d to %d", v, v+1)
		}
		if err := m(doc); err != nil {
			return fmt.Errorf("migrate schema version %d to %d: %w", v, v+1, err)
		}
		doc["schema_version"] = json.Number(fmt.Sprint(v + 1))
	}
	return nil
}
