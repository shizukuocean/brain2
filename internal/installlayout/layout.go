package installlayout

import (
	"errors"
	"path/filepath"
	"strings"
)

const SystemDirectoryName = "Meow System"

var productDirectories = map[string]string{
	"meow-launcher":  "Launcher",
	"brain2-manager": "Brain2 Manager",
	"dev-helper":     "Helper",
}

func ProductDirectoryName(productID string) (string, error) {
	name, ok := productDirectories[strings.TrimSpace(productID)]
	if !ok {
		return "", errors.New("产品没有登记 Meow System 安装目录")
	}
	return name, nil
}

// SystemRootFromSelection turns a user-selected parent directory into the one
// Meow application root. Selecting an existing "Meow System" directory keeps
// it unchanged, which makes the picker safe for both first setup and later
// edits.
func SystemRootFromSelection(selection string) (string, error) {
	selection = filepath.Clean(strings.TrimSpace(selection))
	if selection == "." || !filepath.IsAbs(selection) {
		return "", errors.New("Meow System 位置必须是绝对路径")
	}
	volume := filepath.VolumeName(selection)
	if strings.TrimSpace(volume) == "" || strings.HasPrefix(volume, `\\`) {
		return "", errors.New("Meow System 只能放在本机磁盘")
	}
	if strings.EqualFold(filepath.Base(selection), SystemDirectoryName) {
		return selection, nil
	}
	return filepath.Join(selection, SystemDirectoryName), nil
}

// DirectoryForSystemRoot resolves one product below a user-approved Meow
// System root without discarding a non-root parent such as D:\Apps.
func DirectoryForSystemRoot(systemRoot, productID string) (string, error) {
	systemRoot = filepath.Clean(strings.TrimSpace(systemRoot))
	volume := filepath.VolumeName(systemRoot)
	if systemRoot == "." || !filepath.IsAbs(systemRoot) || strings.TrimSpace(volume) == "" || strings.HasPrefix(volume, `\\`) || !strings.EqualFold(filepath.Base(systemRoot), SystemDirectoryName) {
		return "", errors.New("Meow System 安装根目录无效")
	}
	name, err := ProductDirectoryName(productID)
	if err != nil {
		return "", err
	}
	return filepath.Join(systemRoot, name), nil
}

// SystemRootFromProductDirectory returns the shared root only when the input
// is already the governed directory for the requested product.
func SystemRootFromProductDirectory(directory, productID string) (string, bool) {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if !IsProductDirectory(directory, productID) {
		return "", false
	}
	return filepath.Dir(directory), true
}

// DirectoryFromSelection keeps only the selected volume and applies the one
// governed application layout. The picker is therefore a drive choice, not a
// way to create a second product-location convention.
func DirectoryFromSelection(selection, productID string) (string, error) {
	selection = filepath.Clean(strings.TrimSpace(selection))
	if selection == "." || !filepath.IsAbs(selection) {
		return "", errors.New("安装位置必须是绝对路径")
	}
	volume := filepath.VolumeName(selection)
	if strings.TrimSpace(volume) == "" {
		return "", errors.New("无法从安装位置确定目标磁盘")
	}
	name, err := ProductDirectoryName(productID)
	if err != nil {
		return "", err
	}
	return filepath.Join(volume+string(filepath.Separator), SystemDirectoryName, name), nil
}

// IsProductDirectory also accepts a non-root prefix so the installer core can
// be exercised safely in a temporary directory. Production callers always use
// DirectoryFromSelection and therefore resolve to the selected volume root.
func IsProductDirectory(directory, productID string) bool {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if directory == "." || !filepath.IsAbs(directory) {
		return false
	}
	name, err := ProductDirectoryName(productID)
	if err != nil {
		return false
	}
	parent := filepath.Dir(directory)
	return strings.EqualFold(filepath.Base(directory), name) &&
		strings.EqualFold(filepath.Base(parent), SystemDirectoryName)
}
