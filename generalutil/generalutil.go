// Package generalutil provides general-purpose utility functions.
package generalutil

import (
	"fmt"
	"os"
)

// CheckEnvVars verifies that all given environment variables are set.
func CheckEnvVars(vars []string) error {
	var missing []string
	for _, key := range vars {
		if _, ok := os.LookupEnv(key); !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing env vars: %v", missing)
	}
	return nil
}
