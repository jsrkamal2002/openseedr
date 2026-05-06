package observability

import "os"

// GetEnvOrDefault returns the value of the environment variable key,
// or fallback if it is not set / empty.
func GetEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
