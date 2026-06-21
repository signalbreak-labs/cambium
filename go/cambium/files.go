// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"os"
	"path/filepath"
	"strings"
)

// PathsWithModules returns directories under root that contain files with a
// ".yang" extension. Directories are returned once, in filesystem traversal
// order.
func PathsWithModules(root string) ([]string, error) {
	seen := make(map[string]struct{})
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil || info.IsDir() || !strings.HasSuffix(path, ".yang") {
			return nil
		}
		dir := yangFileParent(path)
		if _, ok := seen[dir]; ok {
			return nil
		}
		seen[dir] = struct{}{}
		paths = append(paths, dir)
		return nil
	})
	return paths, err
}

func yangFileParent(path string) string {
	dir, _ := filepath.Split(path)
	if dir == "" {
		return "."
	}
	return filepath.Clean(dir)
}
