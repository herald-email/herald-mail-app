//go:build !darwin && !linux

package notifications

func newPlatformNotifier(Options) Notifier {
	return noopNotifier{platform: "unsupported"}
}
