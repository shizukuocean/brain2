//go:build !windows

package desktopshortcut

type Request struct {
	TargetExecutable string
	Description      string
}

func Ensure(Request) (string, error) { return "", nil }
func RemoveIfTarget(string) error    { return nil }
