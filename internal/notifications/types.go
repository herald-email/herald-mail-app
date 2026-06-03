package notifications

import (
	"context"
	"runtime"
)

type Request struct {
	ID       string
	Title    string
	Body     string
	DeepLink string
	Sound    bool
}

type Options struct {
	Enabled bool
	Sound   bool
}

type Capabilities struct {
	Delivery   bool
	Activation bool
	Platform   string
}

type Notifier interface {
	Notify(context.Context, Request) error
	Responses() <-chan string
	Capabilities() Capabilities
	Close() error
}

func New(opts Options) Notifier {
	if !opts.Enabled {
		return noopNotifier{platform: runtime.GOOS}
	}
	return newPlatformNotifier(opts)
}

type noopNotifier struct {
	platform string
}

func (n noopNotifier) Notify(context.Context, Request) error { return nil }
func (n noopNotifier) Responses() <-chan string              { return nil }
func (n noopNotifier) Capabilities() Capabilities {
	return Capabilities{Platform: n.platform}
}
func (n noopNotifier) Close() error { return nil }
