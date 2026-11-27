package contract

import (
	"os"
	"path/filepath"
	"testing"
)

// Mirrors testdata/seed/generate.mjs.
const (
	seedAdminID       = "5eed0000-0000-4000-8000-000000000001"
	seedAdminEmail    = "seed-admin@contract.test"
	seedAdminPassword = "SeedPassw0rd!"
	seedAdminPwdv     = 1767225600000 // 2026-01-01T00:00:00.000Z
	seedWebhookSecret = "contract-webhook-secret"
	seedRepo          = "https://github.com/contract-org/contract-repo"
	seedGHMain        = "5eed0000-0000-4000-8000-000000000101"
	seedGHDevelop     = "5eed0000-0000-4000-8000-000000000102"
	seedGHManual      = "5eed0000-0000-4000-8000-000000000103"
	seedApp           = "5eed0000-0000-4000-8000-000000000201"
	seedAppService    = "5eed0000-0000-4000-8000-000000000211"
	seedDepOldTagged  = "5eed0000-0000-4000-8000-000000000301"
	seedDepNoTags     = "5eed0000-0000-4000-8000-000000000302"
	seedDepFailed     = "5eed0000-0000-4000-8000-000000000303"
	seedDepLatest     = "5eed0000-0000-4000-8000-000000000304"
	seedPG            = "5eed0000-0000-4000-8000-000000000401"
	seedRedis         = "5eed0000-0000-4000-8000-000000000402"
	seedMySQL         = "5eed0000-0000-4000-8000-000000000403"
)

// prepareSeeded installs the seed DB and gives seed-app a source directory.
func prepareSeeded(d InstanceDirs) error {
	if err := writeSeedDB(d); err != nil {
		return err
	}
	if err := writeKnownSecrets(d); err != nil {
		return err
	}
	dir := filepath.Join(d.Deployments, seedApp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine:3.20\n"), 0o644)
}

func seededInstance(t *testing.T) *Instance {
	t.Helper()
	return dedicatedInstance(t, BootOptions{Name: "seeded", Prepare: prepareSeeded})
}
