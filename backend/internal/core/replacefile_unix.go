//go:build !windows

package core

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
