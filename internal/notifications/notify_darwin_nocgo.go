//go:build darwin && !cgo

package notifications

func newPlatformNotifier(Options) Notifier {
	return noopNotifier{platform: "darwin"}
}
