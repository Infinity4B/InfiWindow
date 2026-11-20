package hidpi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install writes the override for a monitor, asking for an administrator
// password with the system dialog. It blocks until the dialog is answered.
func Install(vendor, product uint32, sizes []Size) error {
	if len(sizes) == 0 {
		return fmt.Errorf("至少选择一档分辨率")
	}
	if o, err := Read(vendor, product); err != nil {
		return err
	} else if o.Exists && !o.Managed {
		return ErrForeign
	}
	tmp, err := os.CreateTemp("", "infiwindow-override-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(Build(vendor, product, sizes)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	dest := Path(vendor, product)
	script := fmt.Sprintf("/bin/mkdir -p %s && /usr/bin/install -m 644 -o root -g wheel %s %s",
		shellQuote(filepath.Dir(dest)), shellQuote(tmp.Name()), shellQuote(dest))
	return runAsAdmin(script, "InfiWindow 要为外接显示器安装 HiDPI 分辨率。")
}

// Remove deletes the override InfiWindow installed for a monitor.
func Remove(vendor, product uint32) error {
	o, err := Read(vendor, product)
	if err != nil {
		return err
	}
	if !o.Exists {
		return nil
	}
	if !o.Managed {
		return ErrForeign
	}
	dest := Path(vendor, product)
	// rmdir only succeeds when no other override for this vendor is left.
	script := fmt.Sprintf("/bin/rm -f %s; /bin/rmdir %s 2>/dev/null; true",
		shellQuote(dest), shellQuote(filepath.Dir(dest)))
	return runAsAdmin(script, "InfiWindow 要移除外接显示器的 HiDPI 分辨率。")
}

// runAsAdmin runs a shell command as root through the standard macOS
// authorization dialog.
func runAsAdmin(script, prompt string) error {
	as := fmt.Sprintf("do shell script %s with prompt %s with administrator privileges",
		appleScriptQuote(script), appleScriptQuote(prompt))
	out, err := exec.Command("/usr/bin/osascript", "-e", as).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "-128") {
		return ErrCanceled
	}
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("需要管理员权限的操作失败：%s", msg)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func appleScriptQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
