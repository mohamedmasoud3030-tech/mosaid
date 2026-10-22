//go:build !windows

package secrets

import (
	"fmt"
	"os"
)

// checkSecretMode rejects secrets readable by group or other users.
func checkSecretMode(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("secret file permissions too broad: %o", info.Mode().Perm())
	}
	return nil
}
