//go:build !unix

package output

import "os"

// ownedByMe has no portable owner check here; on Windows os.TempDir() is
// per-user already.
func ownedByMe(os.FileInfo) bool { return true }
