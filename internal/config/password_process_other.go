//go:build !unix

package config

import "os/exec"

func configurePasswordProcess(cmd *exec.Cmd) {}
