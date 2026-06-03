//go:build darwin && cgo

package notifications

import (
	"context"
	"testing"
)

func TestDarwinNotifierFallsBackWhenUserNotificationsUnavailable(t *testing.T) {
	originalCanUse := canUseDarwinUserNotifications
	originalInit := initDarwinNotifications
	originalInitialized := darwinInitialized
	originalAvailable := darwinAvailable
	defer func() {
		canUseDarwinUserNotifications = originalCanUse
		initDarwinNotifications = originalInit
		darwinInitialized = originalInitialized
		darwinAvailable = originalAvailable
	}()

	canUseDarwinUserNotifications = func() bool { return true }
	initDarwinNotifications = func() bool { return false }
	darwinInitialized = false
	darwinAvailable = false

	notifier := newPlatformNotifier(Options{Enabled: true})
	if err := notifier.Notify(context.Background(), Request{Title: "Hidden", Body: "No-op"}); err != nil {
		t.Fatalf("fallback Notify returned error: %v", err)
	}
	if caps := notifier.Capabilities(); caps.Platform != "darwin" || caps.Delivery || caps.Activation {
		t.Fatalf("fallback capabilities = %#v, want darwin no-op", caps)
	}
}

func TestDarwinNotifierDoesNotTouchUserNotificationsForUnsupportedLauncher(t *testing.T) {
	originalCanUse := canUseDarwinUserNotifications
	originalInit := initDarwinNotifications
	originalInitialized := darwinInitialized
	originalAvailable := darwinAvailable
	defer func() {
		canUseDarwinUserNotifications = originalCanUse
		initDarwinNotifications = originalInit
		darwinInitialized = originalInitialized
		darwinAvailable = originalAvailable
	}()

	initCalled := false
	canUseDarwinUserNotifications = func() bool { return false }
	initDarwinNotifications = func() bool {
		initCalled = true
		return true
	}
	darwinInitialized = false
	darwinAvailable = false

	notifier := newPlatformNotifier(Options{Enabled: true})
	if initCalled {
		t.Fatal("newPlatformNotifier called UserNotifications init for an unsupported launcher")
	}
	if caps := notifier.Capabilities(); caps.Platform != "darwin" || caps.Delivery || caps.Activation {
		t.Fatalf("fallback capabilities = %#v, want darwin no-op", caps)
	}
}
