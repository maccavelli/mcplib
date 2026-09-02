//go:build !windows

package selfupdate

import (
	"os"
)

func processCleanupReceipt(target Target, root *os.Root) error {
	name := cleanupReceiptName(target.Base)
	_, err := root.Lstat(name)
	if err == nil {
		return os.Remove(cleanupReceiptPath(target))
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
