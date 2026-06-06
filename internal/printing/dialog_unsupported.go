//go:build !darwin || !cgo

package printing

func runNativePrintDialog(_, _ string) (Status, error) {
	return StatusUnsupported, nil
}

func runNativePrintSave(_, _, _ string) (Status, error) {
	return StatusUnsupported, nil
}
