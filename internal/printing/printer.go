package printing

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const helperSubcommand = "__herald-print-dialog"

type SystemPrinter struct {
	HelperPath string
}

func (p SystemPrinter) Print(ctx context.Context, req Request) (Result, error) {
	document, err := BuildHTMLDocument(req)
	if err != nil {
		return Result{}, err
	}
	path, err := WriteTempHTML(document)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(path)

	helper := strings.TrimSpace(p.HelperPath)
	if helper == "" {
		helper, err = os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("locate print helper: %w", err)
		}
	}
	cmd := exec.CommandContext(ctx, helper, helperSubcommand, "--file", path, "--title", requestTitle(req))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, ctx.Err()
		}
		if exit, ok := err.(*exec.ExitError); ok {
			switch exit.ExitCode() {
			case 2:
				return Result{Status: StatusCanceled, Message: "Print canceled", Path: path}, nil
			case 3:
				return Result{Status: StatusUnsupported, Message: "Printing unsupported on this host", Path: path}, nil
			}
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return Result{}, fmt.Errorf("print helper failed: %s", boundedStatus(message))
	}
	return Result{Status: StatusOpened, Message: "Print dialog opened", Path: path}, nil
}

func HandleHelper(args []string) (handled bool, exitCode int, err error) {
	if len(args) == 0 || args[0] != helperSubcommand {
		return false, 0, nil
	}
	fs := flag.NewFlagSet(helperSubcommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "HTML file to print")
	title := fs.String("title", "Herald Email", "print job title")
	savePDF := fs.String("save-pdf", "", "save PDF path for native print smoke tests")
	if err := fs.Parse(args[1:]); err != nil {
		return true, 1, err
	}
	if strings.TrimSpace(*file) == "" {
		return true, 1, fmt.Errorf("missing --file")
	}
	var status Status
	if strings.TrimSpace(*savePDF) != "" {
		status, err = runNativePrintSave(*file, *title, *savePDF)
	} else {
		status, err = runNativePrintDialog(*file, *title)
	}
	if err != nil {
		return true, 1, err
	}
	switch status {
	case StatusCanceled:
		return true, 2, nil
	case StatusUnsupported:
		return true, 3, nil
	default:
		return true, 0, nil
	}
}
