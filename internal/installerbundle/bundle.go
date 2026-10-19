package installerbundle

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "meow-installer-bundle/v1"
const InstalledFilesManifestName = ".meow-installed-files.json"
const manifestName = "meow-package.json"
const trailerSize = 24

var trailerMagic = [16]byte{'M', 'E', 'O', 'W', 'P', 'K', 'G', '1', 'T', 'R', 'A', 'I', 'L', 'E', 'R', '!'}
var productIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	SchemaVersion          string `json:"schema_version"`
	ProductID              string `json:"product_id"`
	Version                string `json:"version"`
	ExecutableRelativePath string `json:"executable_relative_path"`
	SelfUpdate             bool   `json:"self_update"`
	Files                  []File `json:"files"`
}

type BuildOptions struct {
	RunnerPath             string
	PayloadRoot            string
	OutputPath             string
	ProductID              string
	Version                string
	ExecutableRelativePath string
	SelfUpdate             bool
}

type ExtractedFile struct {
	SourcePath   string
	RelativePath string
	SHA256       string
}

type Bundle struct {
	file     *os.File
	archive  *zip.Reader
	Manifest Manifest
	entries  map[string]*zip.File
}

func Build(options BuildOptions) (Manifest, error) {
	for name, value := range map[string]string{"runner": options.RunnerPath, "payload": options.PayloadRoot, "output": options.OutputPath} {
		if !filepath.IsAbs(value) {
			return Manifest{}, fmt.Errorf("%s 路径必须是绝对路径", name)
		}
	}
	if samePath(options.OutputPath, options.RunnerPath) {
		return Manifest{}, errors.New("安装包输出不能覆盖安装程序运行器")
	}
	if err := requireRegularFile(options.RunnerPath); err != nil {
		return Manifest{}, fmt.Errorf("安装程序运行器无效: %w", err)
	}
	rootInfo, err := os.Lstat(filepath.Clean(options.PayloadRoot))
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, errors.New("安装载荷根目录不存在或不是普通目录")
	}

	manifest := Manifest{
		SchemaVersion: SchemaVersion, ProductID: strings.TrimSpace(options.ProductID), Version: strings.TrimSpace(options.Version),
		ExecutableRelativePath: cleanRelative(options.ExecutableRelativePath), SelfUpdate: options.SelfUpdate,
	}
	type sourceFile struct {
		absolute string
		entry    File
	}
	sources := []sourceFile{}
	err = filepath.WalkDir(filepath.Clean(options.PayloadRoot), func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == filepath.Clean(options.PayloadRoot) {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("安装载荷不允许符号链接: %s", filename)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("安装载荷包含特殊文件: %s", filename)
		}
		relative, relErr := filepath.Rel(options.PayloadRoot, filename)
		if relErr != nil {
			return relErr
		}
		relative = cleanRelative(relative)
		if err := validateRelative(relative); err != nil {
			return err
		}
		digest, hashErr := fileSHA256(filename)
		if hashErr != nil {
			return hashErr
		}
		sources = append(sources, sourceFile{absolute: filename, entry: File{Path: relative, SHA256: digest, Size: info.Size()}})
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].entry.Path < sources[j].entry.Path })
	for _, source := range sources {
		manifest.Files = append(manifest.Files, source.entry)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}

	outputDirectory := filepath.Dir(filepath.Clean(options.OutputPath))
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return Manifest{}, err
	}
	archiveFile, err := os.CreateTemp(outputDirectory, ".meow-package-*.zip")
	if err != nil {
		return Manifest{}, err
	}
	archivePath := archiveFile.Name()
	defer os.Remove(archivePath)
	zipWriter := zip.NewWriter(archiveFile)
	manifestHeader := &zip.FileHeader{Name: manifestName, Method: zip.Store}
	manifestHeader.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
	manifestWriter, err := zipWriter.CreateHeader(manifestHeader)
	if err == nil {
		var encoded []byte
		encoded, err = json.MarshalIndent(manifest, "", "  ")
		if err == nil {
			_, err = manifestWriter.Write(append(encoded, '\n'))
		}
	}
	for _, source := range sources {
		if err != nil {
			break
		}
		header := &zip.FileHeader{Name: "payload/" + source.entry.Path, Method: zip.Store}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		writer, createErr := zipWriter.CreateHeader(header)
		if createErr != nil {
			err = createErr
			break
		}
		input, openErr := os.Open(source.absolute)
		if openErr != nil {
			err = openErr
			break
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			err = copyErr
		} else if closeErr != nil {
			err = closeErr
		}
	}
	if closeErr := zipWriter.Close(); err == nil {
		err = closeErr
	}
	if syncErr := archiveFile.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := archiveFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Manifest{}, err
	}

	temporary, err := os.CreateTemp(outputDirectory, ".meow-installer-*.exe")
	if err != nil {
		return Manifest{}, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = appendFile(temporary, options.RunnerPath); err == nil {
		err = appendFile(temporary, archivePath)
	}
	archiveInfo, statErr := os.Stat(archivePath)
	if err == nil {
		err = statErr
	}
	if err == nil {
		trailer := make([]byte, trailerSize)
		copy(trailer[:16], trailerMagic[:])
		binary.LittleEndian.PutUint64(trailer[16:], uint64(archiveInfo.Size()))
		_, err = temporary.Write(trailer)
	}
	if syncErr := temporary.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Manifest{}, err
	}
	if err := replaceFile(temporaryPath, filepath.Clean(options.OutputPath)); err != nil {
		return Manifest{}, err
	}
	_ = os.Chmod(options.OutputPath, 0o755)
	return manifest, nil
}

func Open(filename string) (*Bundle, error) {
	if err := requireRegularFile(filename); err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil || info.Size() < trailerSize {
		return nil, errors.New("安装程序没有有效的内置载荷")
	}
	trailer := make([]byte, trailerSize)
	if _, err := file.ReadAt(trailer, info.Size()-trailerSize); err != nil {
		return nil, err
	}
	if string(trailer[:16]) != string(trailerMagic[:]) {
		return nil, errors.New("安装程序载荷标记无效")
	}
	archiveSize := int64(binary.LittleEndian.Uint64(trailer[16:]))
	archiveOffset := info.Size() - trailerSize - archiveSize
	if archiveSize <= 0 || archiveOffset <= 0 {
		return nil, errors.New("安装程序载荷范围无效")
	}
	archive, err := zip.NewReader(io.NewSectionReader(file, archiveOffset, archiveSize), archiveSize)
	if err != nil {
		return nil, errors.New("安装程序载荷无法读取")
	}
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		if _, duplicate := entries[entry.Name]; duplicate {
			return nil, errors.New("安装程序载荷包含重复文件")
		}
		entries[entry.Name] = entry
	}
	manifestEntry := entries[manifestName]
	if manifestEntry == nil || manifestEntry.UncompressedSize64 > 1<<20 {
		return nil, errors.New("安装程序缺少有效清单")
	}
	reader, err := manifestEntry.Open()
	if err != nil {
		return nil, err
	}
	manifestBytes, readErr := io.ReadAll(io.LimitReader(reader, 1<<20+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, errors.New("安装程序清单 JSON 无效")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if len(entries) != len(manifest.Files)+1 {
		return nil, errors.New("安装程序包含未登记文件")
	}
	for _, listed := range manifest.Files {
		entry := entries["payload/"+listed.Path]
		if entry == nil || int64(entry.UncompressedSize64) != listed.Size || entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("安装程序文件与清单不一致: %s", listed.Path)
		}
	}
	failed = false
	return &Bundle{file: file, archive: archive, Manifest: manifest, entries: entries}, nil
}

func (b *Bundle) Close() error {
	if b == nil || b.file == nil {
		return nil
	}
	return b.file.Close()
}

func (b *Bundle) Extract(destination string) ([]ExtractedFile, error) {
	if b == nil || b.archive == nil || !filepath.IsAbs(destination) || isBroadRoot(destination) {
		return nil, errors.New("安装载荷解压目录无效")
	}
	if err := os.MkdirAll(filepath.Clean(destination), 0o700); err != nil {
		return nil, err
	}
	extracted := make([]ExtractedFile, 0, len(b.Manifest.Files))
	for _, listed := range b.Manifest.Files {
		entry := b.entries["payload/"+listed.Path]
		target := filepath.Join(destination, filepath.FromSlash(listed.Path))
		if !pathInside(target, destination) {
			return nil, errors.New("安装载荷路径越界")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return nil, err
		}
		input, err := entry.Open()
		if err != nil {
			return nil, err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		if err != nil {
			input.Close()
			return nil, err
		}
		digest := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(input, listed.Size+1))
		syncErr := output.Sync()
		outputCloseErr := output.Close()
		inputCloseErr := input.Close()
		if copyErr != nil || syncErr != nil || outputCloseErr != nil || inputCloseErr != nil || written != listed.Size || fmt.Sprintf("%x", digest.Sum(nil)) != listed.SHA256 {
			_ = os.Remove(target)
			return nil, fmt.Errorf("安装载荷校验失败: %s", listed.Path)
		}
		extracted = append(extracted, ExtractedFile{SourcePath: target, RelativePath: listed.Path, SHA256: listed.SHA256})
	}
	return extracted, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion || !productIDPattern.MatchString(m.ProductID) || strings.TrimSpace(m.Version) == "" {
		return errors.New("安装程序清单身份无效")
	}
	if err := validateRelative(m.ExecutableRelativePath); err != nil || !strings.EqualFold(path.Ext(m.ExecutableRelativePath), ".exe") {
		return errors.New("安装程序入口必须是载荷内的相对 EXE 路径")
	}
	if m.SelfUpdate && m.ProductID != "meow-launcher" {
		return errors.New("只有 Meow Launcher 安装包可以声明自更新")
	}
	if len(m.Files) == 0 {
		return errors.New("安装程序没有载荷文件")
	}
	seen := map[string]bool{}
	entrypointFound := false
	for _, file := range m.Files {
		if err := validateRelative(file.Path); err != nil || !sha256Pattern.MatchString(file.SHA256) || file.Size < 0 {
			return errors.New("安装程序文件记录无效")
		}
		if strings.EqualFold(file.Path, InstalledFilesManifestName) {
			return errors.New("安装载荷不能覆盖 Launcher 管理的文件清单")
		}
		key := strings.ToLower(file.Path)
		if seen[key] {
			return errors.New("安装程序文件记录重复")
		}
		seen[key] = true
		if strings.EqualFold(file.Path, m.ExecutableRelativePath) {
			entrypointFound = true
		}
	}
	if !entrypointFound {
		return errors.New("安装程序载荷缺少产品入口")
	}
	return nil
}

func validateRelative(value string) error {
	if cleanRelative(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") || path.IsAbs(value) || strings.Contains(value, ":") {
		return fmt.Errorf("安装载荷相对路径无效: %s", value)
	}
	return nil
}

func cleanRelative(value string) string {
	return path.Clean(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
}

func pathInside(candidate, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func isBroadRoot(value string) bool {
	clean := filepath.Clean(value)
	volume := filepath.VolumeName(clean)
	return clean == string(filepath.Separator) || clean == "." || (volume != "" && samePath(clean, volume+string(filepath.Separator)))
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func requireRegularFile(filename string) error {
	info, err := os.Lstat(filepath.Clean(filename))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("路径不是普通文件")
	}
	return nil
}

func fileSHA256(filename string) (string, error) {
	input, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return "", err
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func appendFile(output io.Writer, filename string) error {
	input, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return err
	}
	defer input.Close()
	_, err = io.Copy(output, input)
	return err
}

func replaceFile(temporaryPath, finalPath string) error {
	backupPath := temporaryPath + ".previous"
	hadExisting := false
	if info, err := os.Lstat(finalPath); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("已有安装包输出不是普通文件")
		}
		if err := os.Rename(finalPath, backupPath); err != nil {
			return err
		}
		hadExisting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		if hadExisting {
			_ = os.Rename(backupPath, finalPath)
		}
		return err
	}
	if hadExisting {
		_ = os.Remove(backupPath)
	}
	return nil
}
