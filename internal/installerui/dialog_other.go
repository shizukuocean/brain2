//go:build !windows

package installerui

import "errors"

func ChooseFolder(string) (string, bool, error) {
	return "", false, errors.New("interactive Launcher setup is only available on Windows")
}

func ChooseFolderFrom(title, _ string) (string, bool, error) { return ChooseFolder(title) }

func Confirm(string, string) bool { return false }
func ShowError(string, string)    {}
func ShowInfo(string, string)     {}
