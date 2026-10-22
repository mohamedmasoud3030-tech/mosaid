//go:build windows

package secrets

import "os"

// checkSecretMode is a no-op on Windows: NTFS ACLs, not POSIX mode bits, decide
// who can read a file, and Go reports every readable file as 0666. The runtime
// therefore relies on the key living under the user's own profile (%LOCALAPPDATA%
// is not readable by other standard users) plus Mosaid never logging it.
// Documenting this is preferable to silently accepting a broad file on Unix.
func checkSecretMode(os.FileInfo) error { return nil }
