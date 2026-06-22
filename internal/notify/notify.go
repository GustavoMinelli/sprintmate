// Package notify sends best-effort completion notifications for autonomous runs:
// a terminal bell, a desktop notification, and/or a webhook POST. Every channel
// is opt-in and failures are swallowed — a notification never blocks or crashes
// the TUI.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Options describes one notification and which channels to use.
type Options struct {
	Title   string
	Body    string
	Bell    bool
	OS      bool
	Webhook string
}

// Send dispatches the notification over the enabled channels. It is best-effort
// and returns nothing: each channel's failure is ignored.
func Send(ctx context.Context, o Options) {
	if o.Bell {
		// The bell goes to stderr so it isn't captured by the alt-screen buffer.
		fmt.Fprint(os.Stderr, "\a")
	}
	if o.OS {
		osNotify(ctx, o.Title, o.Body)
	}
	if strings.TrimSpace(o.Webhook) != "" {
		postWebhook(ctx, o.Webhook, o.Title, o.Body)
	}
}

// osNotify shows a desktop notification using the platform's native tool. It is
// a no-op when no supported mechanism is available.
func osNotify(ctx context.Context, title, body string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", body, title)
		cmd = exec.CommandContext(ctx, "osascript", "-e", script)
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return
		}
		cmd = exec.CommandContext(ctx, "notify-send", title, body)
	default:
		return // Windows toast/others: bell + webhook still apply.
	}
	_ = cmd.Run()
}

func postWebhook(ctx context.Context, url, title, body string) {
	payload, err := json.Marshal(map[string]string{"title": title, "body": body})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}
