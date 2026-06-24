// Package generalutil provides general-purpose utility functions.
package generalutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

func LoadEnv() error {
	// Define command line flag
	env := flag.String("env", "development", "Environment (development, staging, production)")
	flag.Parse()

	// Load environment-specific .env file
	envFile := ".env." + *env
	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: Could not load %s file: %v", envFile, err)
		return err
	}

	// Your app code here
	log.Printf("Loading env file %s (%s mode)", envFile, *env)

	return nil
}

// resolvePath converts a relative path (e.g., "./certs/ca-cert.pem")
// into an absolute path based on the executable's directory.
// If the path is already absolute, it returns it unchanged.
func ResolvePath(relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	}

	// Use working directory instead of executable path
	// (works correctly with both `go run` and compiled binaries)
	cwd, err := os.Getwd()
	if err != nil {
		return relPath
	}

	cleanRel := strings.TrimPrefix(relPath, ".")
	cleanRel = strings.TrimPrefix(cleanRel, string(os.PathSeparator))
	return filepath.Join(cwd, cleanRel)
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}
