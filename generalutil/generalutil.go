// Package generalutil provides general-purpose utility functions.
package generalutil

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
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

func LoadEnv() error {
	// Define command line flag
	env := flag.String("env", "development", "Environment (development, staging, production)")
	flag.Parse()

	// Load environment-specific .env file
	envFile := ".env." + *env
	if err := godotenv.Load(envFile); err != nil {
		fmt.Errorf("Warning: Could not load %s file", envFile)
		return err
	}

	// Set APP_ENV for other parts of the app
	if err := os.Setenv("APP_ENV", *env); err != nil {
		fmt.Errorf("Could not set env var APP_ENV: %v", err)
		return err
	}

	// Your app code here
	log.Printf("Starting app in %s mode", *env)

	return nil
}
