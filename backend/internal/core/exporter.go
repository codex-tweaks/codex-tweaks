package core

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

type PackageExporter struct {
	mu sync.Mutex
}

func NewPackageExporter() *PackageExporter { return &PackageExporter{} }

func PackageArchiveFileName(pkg Package) string {
	name := archiveNameComponent(pkg.DisplayName())
	if pkg.Manifest == nil {
		return name + ".zip"
	}
	version := archiveNameComponent(NormalizeVersion(pkg.Manifest.Version))
	return name + "-v" + version + ".zip"
}

func packageArchiveRootName(pkg Package) string {
	return archiveNameComponent(pkg.DisplayName())
}

func archiveNameComponent(value string) string {
	const maximumBytes = 80
	component := destinationDirectoryName(value)
	if len([]byte(component)) <= maximumBytes {
		return component
	}
	suffix := "-" + FingerprintString(value)[:8]
	limit := maximumBytes - len(suffix)
	for len([]byte(component)) > limit {
		_, size := utf8.DecodeLastRuneInString(component)
		component = component[:len(component)-size]
	}
	component = strings.Trim(component, ".-_")
	if component == "" {
		component = "package"
	}
	return component + suffix
}

func (e *PackageExporter) Export(ctx context.Context, pkg Package, destinationPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if strings.TrimSpace(destinationPath) == "" || !filepath.IsAbs(destinationPath) {
		return errors.New("请选择有效的 ZIP 保存位置。")
	}
	if !strings.EqualFold(filepath.Ext(destinationPath), ".zip") {
		return errors.New("导出文件必须使用 .zip 扩展名。")
	}
	source := filepath.Clean(pkg.Directory)
	destination := filepath.Clean(destinationPath)
	rootInfo, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("无法读取功能包目录：%w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("功能包目录无效或是符号链接。")
	}
	if pathIsWithin(destination, source) {
		return errors.New("ZIP 不能保存到正在导出的功能包目录中。")
	}
	parent := filepath.Dir(destination)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return errors.New("ZIP 保存目录不存在或不可用。")
	}

	temporary, err := os.CreateTemp(parent, ".codex-tweaks-export-*.zip")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		_ = temporary.Close()
		if !keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if err := writePackageArchive(ctx, temporary, source, packageArchiveRootName(pkg)); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	archiveInfo, err := os.Stat(temporaryPath)
	if err != nil {
		return err
	}
	if archiveInfo.Size() > MaximumArchiveSize {
		return fmt.Errorf("导出的 ZIP 大小超过上限（%d 字节）。", MaximumArchiveSize)
	}
	if err := replaceFile(temporaryPath, destination); err != nil {
		return err
	}
	keepTemporary = true
	return nil
}

func writePackageArchive(ctx context.Context, output io.Writer, source, rootName string) error {
	archive := zip.NewWriter(output)
	fileCount := 0
	expandedSize := int64(0)
	walkErr := filepath.WalkDir(source, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, filePath)
		if err != nil || relative == "." {
			return err
		}
		relativeSlash := filepath.ToSlash(relative)
		first := strings.Split(relativeSlash, "/")[0]
		if ignoredPackageRoots[first] {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == ".DS_Store" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("功能包不能包含符号链接：%s", relativeSlash)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("功能包包含不支持的特殊文件：%s", relativeSlash)
		}
		fileCount++
		expandedSize += info.Size()
		if fileCount > MaximumPackageFileCount {
			return fmt.Errorf("功能包文件数量超过上限（%d 个）。", MaximumPackageFileCount)
		}
		if expandedSize > MaximumExpandedSize {
			return fmt.Errorf("功能包大小超过上限（%d 字节）。", MaximumExpandedSize)
		}
		entryName := path.Join(rootName, relativeSlash)
		if err := validateArchivePath(entryName); err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = entryName
		header.Method = zip.Deflate
		header.SetMode(info.Mode())
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(filePath)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written != info.Size() {
			return fmt.Errorf("功能包文件在导出期间发生变化，请重试：%s", relativeSlash)
		}
		return nil
	})
	if walkErr != nil {
		_ = archive.Close()
		return walkErr
	}
	if fileCount == 0 {
		_ = archive.Close()
		return errors.New("功能包目录为空，无法导出。")
	}
	return archive.Close()
}
