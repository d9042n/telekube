//go:build e2e

package e2e

import (
	"os"
	"strings"
)

// getenv wraps os.Getenv for testability and cross-platform consistency.
func getenv(key string) string {
	return os.Getenv(key)
}

// IsSkipCluster returns true when E2E_SKIP_CLUSTER=true env var is set.
// This is exported so tests in the e2e_test package can use it to skip k3s tests.
func IsSkipCluster() bool {
	return skipCluster()
}

// skipCluster returns true when E2E_SKIP_CLUSTER=true env var is set.
func skipCluster() bool {
	v := getenv("E2E_SKIP_CLUSTER")
	return strings.EqualFold(v, "true") || v == "1"
}

// writeFileWithPerm writes data to path with the given file permission bits.
func writeFileWithPerm(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// readFileBytes reads all bytes from path.
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
