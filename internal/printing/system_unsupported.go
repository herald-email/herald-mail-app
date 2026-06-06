//go:build !darwin || !cgo

package printing

func NewSystemPrinter() Printer {
	return UnsupportedPrinter{Reason: "macOS printing requires a local Darwin+cgo build"}
}
