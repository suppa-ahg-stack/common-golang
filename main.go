package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func checkHaveEnvVars(envVarsToCheck []string) error {
	var missingEnvVars []string
	for _, key := range envVarsToCheck {
		if val, ok := os.LookupEnv(key); !ok {
			missingEnvVars = append(missingEnvVars, val)
		}
	}

	if len(missingEnvVars) > 0 {
		return fmt.Errorf("These env vars are missing: %v", missingEnvVars)
	}

	return nil
}

func createServer(handler *http.ServeMux) *http.Server {
	serverAddEnvName := "WEB_SERVER_ADDRESS"
	serverPortEnvName := "WEB_SERVER_PORT"
	if err := checkHaveEnvVars([]string{serverAddEnvName, serverPortEnvName}); err != nil {
		log.Fatalf("Envrironment variables are missing %v", err)
	}

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%s", os.Getenv(serverAddEnvName), os.Getenv(serverPortEnvName)),
		Handler: handler,
	}
}

func runServer() error {}

func main() {
	server := createServer()

	if err := runServer(); err != nil {
		log.Fatalf("Server error %v", err)
	}
}
