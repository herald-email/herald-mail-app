//go:build linux

package notifications

import (
	"context"
	"os/exec"
)

type platformNotifier struct {
	opts Options
}

func newPlatformNotifier(opts Options) Notifier {
	return platformNotifier{opts: opts}
}

func (n platformNotifier) Notify(ctx context.Context, req Request) error {
	return exec.CommandContext(ctx, "notify-send", req.Title, req.Body).Run()
}

func (n platformNotifier) Responses() <-chan string { return nil }
func (n platformNotifier) Capabilities() Capabilities {
	return Capabilities{Delivery: true, Activation: false, Platform: "linux"}
}
func (n platformNotifier) Close() error { return nil }
