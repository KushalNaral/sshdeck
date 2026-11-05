package vpn

import (
	"golang.org/x/sys/windows/registry"
)

const (
	nsisBase    = `Software\Microsoft\Windows\CurrentVersion\Uninstall\OpenConnect-GUI`
	nsisParent  = nsisBase + `\Components`
	nsisConsole = nsisParent + `\App_Console`
)

// selectConsoleComponent makes the silent OpenConnect-GUI install include the console component by setting the
// "Installed" flag the installer reads for it. The returned function puts the registry back as it was (deleting what
// this call created), for when the install fails.
func selectConsoleComponent() (func(), error) {
	exists := func(path string) bool {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
		if err != nil {
			return false
		}
		k.Close()
		return true
	}
	hadBase, hadParent, hadConsole := exists(nsisBase), exists(nsisParent), exists(nsisConsole)
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, nsisConsole, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	prev, _, perr := k.GetIntegerValue("Installed")
	if err := k.SetDWordValue("Installed", 1); err != nil {
		return nil, err
	}
	return func() {
		switch {
		case hadConsole && perr == nil:
			if k, err := registry.OpenKey(registry.LOCAL_MACHINE, nsisConsole, registry.SET_VALUE); err == nil {
				_ = k.SetDWordValue("Installed", uint32(prev))
				k.Close()
			}
		case hadConsole:
			if k, err := registry.OpenKey(registry.LOCAL_MACHINE, nsisConsole, registry.SET_VALUE); err == nil {
				_ = k.DeleteValue("Installed")
				k.Close()
			}
		default:
			_ = registry.DeleteKey(registry.LOCAL_MACHINE, nsisConsole)
			if !hadParent {
				_ = registry.DeleteKey(registry.LOCAL_MACHINE, nsisParent)
				if !hadBase {
					_ = registry.DeleteKey(registry.LOCAL_MACHINE, nsisBase)
				}
			}
		}
	}, nil
}
