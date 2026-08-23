package core

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	MaximumPackageFileCount = 20_000
	MaximumExpandedSize     = int64(1_073_741_824)
	MaximumArchiveSize      = int64(536_870_912)
)

var ignoredPackageRoots = map[string]bool{".git": true, "node_modules": true, "__MACOSX": true}

type LocalPackageInstallResult struct {
	PackageID string          `json:"packageID"`
	Manifest  PackageManifest `json:"manifest"`
	Directory string          `json:"directory"`
}

type LocalInstaller struct {
	store *Store
	mu    sync.Mutex
}

func NewLocalInstaller(store *Store) *LocalInstaller { return &LocalInstaller{store: store} }

func (i *LocalInstaller) Install(source string) (LocalPackageInstallResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.store.Prepare(); err != nil {
		return LocalPackageInstallResult{}, err
	}
	container, err := os.MkdirTemp(i.store.PackagesDirectory, ".installing-")
	if err != nil {
		return LocalPackageInstallResult{}, err
	}
	defer os.RemoveAll(container)

	info, err := os.Stat(source)
	if err != nil {
		return LocalPackageInstallResult{}, err
	}
	var packageSource string
	if info.IsDir() {
		packageSource, err = locatePackageRoot(source)
	} else if info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(source), ".zip") {
		if info.Size() > MaximumArchiveSize {
			return LocalPackageInstallResult{}, fmt.Errorf("功能包压缩包大小超过上限（%d 字节）。", MaximumArchiveSize)
		}
		extraction := filepath.Join(container, "extracted")
		if err = extractSafeZIP(source, extraction); err == nil {
			packageSource, err = locatePackageRoot(extraction)
		}
	} else {
		err = errors.New("请选择一个功能包目录或 ZIP 压缩包。")
	}
	if err != nil {
		return LocalPackageInstallResult{}, err
	}

	staged := filepath.Join(container, "package")
	if err := copySafePackageTree(packageSource, staged); err != nil {
		return LocalPackageInstallResult{}, err
	}
	inspected := i.store.InspectPackage(staged, LocalOrigin(), nil, "")
	if inspected.ValidationError != nil || inspected.Manifest == nil {
		message := "无法读取 package.json。"
		if inspected.ValidationError != nil {
			message = *inspected.ValidationError
		}
		return LocalPackageInstallResult{}, errors.New("不是有效的 Codex Tweaks 功能包：" + message)
	}
	existingPackages, err := i.store.LoadPackages()
	if err != nil {
		return LocalPackageInstallResult{}, err
	}
	for _, pkg := range existingPackages {
		if pkg.Manifest != nil && pkg.Manifest.Name == inspected.Manifest.Name {
			return LocalPackageInstallResult{}, fmt.Errorf("功能包 %s 已经安装。", inspected.Manifest.Name)
		}
	}
	directoryName := destinationDirectoryName(inspected.Manifest.Name)
	destination := filepath.Join(i.store.PackagesDirectory, directoryName)
	if _, err := os.Stat(destination); err == nil {
		return LocalPackageInstallResult{}, fmt.Errorf("本地 packages 目录中已经存在目录：%s", directoryName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return LocalPackageInstallResult{}, err
	}
	if err := os.Rename(staged, destination); err != nil {
		return LocalPackageInstallResult{}, err
	}
	return LocalPackageInstallResult{PackageID: inspected.Manifest.Name, Manifest: *inspected.Manifest, Directory: destination}, nil
}

func locatePackageRoot(root string) (string, error) {
	if isRegularNonSymlink(filepath.Join(root, "package.json")) {
		return root, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	for _, entry := range entries {
		if entry.Name() == ".DS_Store" || ignoredPackageRoots[entry.Name()] || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if isRegularNonSymlink(filepath.Join(candidate, "package.json")) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) > 1 {
		return "", errors.New("检测到多个包含 package.json 的目录，无法确定要安装哪个功能包。")
	}
	if len(candidates) == 0 {
		return "", errors.New("没有找到 package.json；它必须位于所选目录或 ZIP 的唯一一级目录中。")
	}
	return candidates[0], nil
}

func extractSafeZIP(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return errors.New("ZIP 压缩包无效：" + err.Error())
	}
	defer archive.Close()
	if len(archive.File) == 0 {
		return errors.New("ZIP 压缩包无效：压缩包为空。")
	}
	if len(archive.File) > MaximumPackageFileCount {
		return fmt.Errorf("功能包文件数量超过上限（%d 个）。", MaximumPackageFileCount)
	}
	var expandedSize uint64
	for _, entry := range archive.File {
		if err := validateArchivePath(entry.Name); err != nil {
			return err
		}
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("功能包不能包含符号链接：%s", entry.Name)
		}
		if !entry.FileInfo().IsDir() && !mode.IsRegular() {
			return fmt.Errorf("功能包包含不支持的特殊文件：%s", entry.Name)
		}
		expandedSize += entry.UncompressedSize64
		if expandedSize > uint64(MaximumExpandedSize) {
			return fmt.Errorf("功能包解压后大小超过上限（%d 字节）。", MaximumExpandedSize)
		}
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	for _, entry := range archive.File {
		relative := strings.TrimSuffix(entry.Name, "/")
		if relative == "" {
			continue
		}
		first := strings.Split(relative, "/")[0]
		if ignoredPackageRoots[first] || filepath.Base(filepath.FromSlash(relative)) == ".DS_Store" {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if !pathIsWithin(target, destination) {
			return fmt.Errorf("ZIP 包含不安全的路径：%s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, int64(entry.UncompressedSize64)+1))
		inputCloseErr, outputCloseErr := input.Close(), output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		if outputCloseErr != nil {
			return outputCloseErr
		}
	}
	return nil
}

func validateArchivePath(path string) error {
	normalized := strings.TrimRight(path, "/")
	components := strings.Split(normalized, "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") || strings.Contains(normalized, `\`) || strings.ContainsRune(normalized, 0) {
		return fmt.Errorf("ZIP 包含不安全的路径：%s", path)
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("ZIP 包含不安全的路径：%s", path)
		}
	}
	return nil
}

func copySafePackageTree(source, destination string) error {
	rootInfo, err := os.Lstat(source)
	if err != nil || !rootInfo.IsDir() {
		return errors.New("没有找到 package.json；它必须位于所选目录或 ZIP 的唯一一级目录中。")
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("功能包不能包含符号链接：%s", filepath.Base(source))
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	fileCount, expandedSize := 0, int64(0)
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
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
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
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
			return fmt.Errorf("功能包解压后大小超过上限（%d 字节）。", MaximumExpandedSize)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr, outputCloseErr := input.Close(), output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return outputCloseErr
	})
}

func destinationDirectoryName(packageID string) string {
	isAllowed := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune("._-", r) }
	safe := !strings.HasPrefix(packageID, ".") && len([]byte(packageID)) <= 120
	for _, value := range packageID {
		if !isAllowed(value) {
			safe = false
			break
		}
	}
	if safe {
		return packageID
	}
	var slug strings.Builder
	for _, value := range packageID {
		if isAllowed(value) {
			slug.WriteRune(value)
		} else {
			slug.WriteByte('-')
		}
	}
	value := slug.String()
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, ".-_")
	if value == "" {
		value = "package"
	}
	for len([]rune(value)) > 80 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value + "-" + FingerprintString(packageID)[:8]
}
