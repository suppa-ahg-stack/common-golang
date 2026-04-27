// Package generalutil provides general-purpose utility functions.
package generalutil

import (
	"flag"
	"fmt"
	"log"

	"github.com/joho/godotenv"
)

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

	// Your app code here
	log.Printf("Loading env file %s (%s mode)", envFile, *env)

	return nil
}
