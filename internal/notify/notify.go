// Package notify sends OS notifications (Q-72), directly, since not every terminal handles
// them. Text is always passed as data, so a session's name can't run as a command.
package notify

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// ErrUnavailable means this system has nothing to show a notification with.
var ErrUnavailable = errors.New("no way to show notifications here")

const macScript = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`

const windowsScript = `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $template.GetElementsByTagName('text')
$text.Item(0).AppendChild($template.CreateTextNode($env:LAZYCODE_NOTIFY_TITLE)) | Out-Null
$text.Item(1).AppendChild($template.CreateTextNode($env:LAZYCODE_NOTIFY_BODY)) | Out-Null
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('LazyCode').Show([Windows.UI.Notifications.ToastNotification]::new($template))`

// Send shows a notification. It can block for a moment, so callers shouldn't use it on the
// UI goroutine.
func Send(title, body string) error {
	title, body = clean(title), clean(body)
	var command []string
	var env []string
	switch runtime.GOOS {
	case "darwin":
		command = []string{"osascript", "-e", macScript, title, body}
	case "windows":
		command = []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", windowsScript}
		env = []string{"LAZYCODE_NOTIFY_TITLE=" + title, "LAZYCODE_NOTIFY_BODY=" + body}
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return ErrUnavailable // Over SSH, for example.
		}
		command = []string{"notify-send", "--app-name=LazyCode", "--", title, body}
	}
	path, err := exec.LookPath(command[0])
	if err != nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, command[1:]...)
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

func clean(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}
