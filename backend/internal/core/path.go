package core

import "path/filepath"

func joinPath(base, name string) string { return filepath.Join(base, name) }
