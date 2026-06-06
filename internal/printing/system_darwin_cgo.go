//go:build darwin && cgo

package printing

func NewSystemPrinter() Printer {
	return SystemPrinter{}
}
