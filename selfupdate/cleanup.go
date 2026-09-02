package selfupdate

import "path/filepath"

func cleanupReceiptName(base string) string {
	return "." + base + ".selfupdate.cleanup"
}

func cleanupReceiptPath(target Target) string {
	return filepath.Join(target.Dir, cleanupReceiptName(target.Base))
}
