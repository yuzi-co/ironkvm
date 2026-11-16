// Package atomicfile replaces files whole.
package atomicfile

import (
	"os"
	"path/filepath"
)

// WriteFile replaces path with data so that a reader, or a power cut,
// sees either the old file or the new one and never a truncated mix. The data
// goes to a hidden temporary file in the same directory, which is synced and
// renamed over path, and the directory is then synced so the rename itself
// survives a power cut. The temporary file is removed on any failure.
//
// The file gets mode exactly, whatever the umask. The temporary file is
// created 0600 and changed to mode before anything is written, so a secret is
// never readable by others, not even for a moment. A filesystem without
// permission bits, such as exFAT on /data, refuses the change; that is not an
// error, and the file then has whatever the mount gives it.
//
// The directory must exist. Renaming over an existing file works on exFAT
// under Linux as well as on ext4.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	_ = tmp.Chmod(mode)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	renamed = true

	// Not every filesystem can sync a directory, so this is best effort.
	_ = SyncDir(dir)
	return nil
}

// SyncDir writes the directory dir itself to its medium, so that a file
// created in it, renamed into it or removed from it stays that way after a
// power cut. Without it the change to the directory is only in the page
// cache. FAT on /boot and exFAT on /data have no journal, so a power cut
// before the writeback rolls the entry back: a deleted image is there again,
// and one that finished uploading is gone.
//
// On exFAT and FAT this is not a small write. The sync also writes the
// device's other dirty metadata and flushes the card's cache, so it can take
// as long as a flush of the card. A caller must not hold a lock that others
// wait on across it, and one that changes several entries of a directory
// syncs once at the end.
//
// The file's own bytes are not covered: sync the file before the rename that
// publishes it.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}
