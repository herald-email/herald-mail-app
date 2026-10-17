package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const passwordCommandTimeout = 30 * time.Second
const maxPasswordOutput = 64 * 1024

// PasswordCommand holds either a shell string or an argv array, never both.
// Treat it as immutable so source/config copies can safely share it.
type PasswordCommand struct {
	Shell string
	Args  []string
}

// Formatting credentials must not reveal either passwords or command arguments.
func (c CredentialsConfig) String() string   { return "[redacted credentials]" }
func (c CredentialsConfig) GoString() string { return c.String() }
func (c PasswordCommand) String() string     { return "[redacted password command]" }
func (c PasswordCommand) GoString() string   { return c.String() }

func (c *CredentialsConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Username string    `yaml:"username"`
		Password yaml.Node `yaml:"password"`
	}
	if err := node.Decode(&raw); err != nil {
		return errors.New("invalid credentials configuration")
	}
	next := CredentialsConfig{Username: raw.Username}
	p := &raw.Password
	if p.Kind == yaml.AliasNode {
		p = p.Alias
	}
	switch p.Kind {
	case 0:
	case yaml.ScalarNode:
		// Preserve the legacy scalar decoding behavior, including numeric passwords.
		if err := p.Decode(&next.Password); err != nil {
			return errors.New("credentials.password must be a scalar or command mapping")
		}
	case yaml.MappingNode:
		if len(p.Content) != 2 || p.Content[0].Value != "command" {
			return errors.New("credentials.password must contain only command")
		}
		command := &PasswordCommand{}
		value := p.Content[1]
		if value.Kind == yaml.AliasNode {
			value = value.Alias
		}
		switch value.Kind {
		case yaml.ScalarNode:
			if value.Tag != "!!str" {
				return errors.New("credentials.password.command must be a string or string array")
			}
			command.Shell = value.Value
		case yaml.SequenceNode:
			for _, arg := range value.Content {
				if arg.Kind != yaml.ScalarNode || arg.Tag != "!!str" {
					return errors.New("credentials.password.command arguments must be strings")
				}
				command.Args = append(command.Args, arg.Value)
			}
		default:
			return errors.New("credentials.password.command must be a string or string array")
		}
		if err := command.validate(); err != nil {
			return err
		}
		next.PasswordCommand = command
	default:
		return errors.New("credentials.password must be a scalar or command mapping")
	}
	*c = next
	return nil
}

func (c CredentialsConfig) MarshalYAML() (any, error) {
	var password any = c.Password
	if c.PasswordCommand != nil {
		if err := c.ValidatePassword(); err != nil {
			return nil, err
		}
		var command any = c.PasswordCommand.Shell
		if c.PasswordCommand.Args != nil {
			command = c.PasswordCommand.Args
		}
		password = struct {
			Command any `yaml:"command"`
		}{command}
	}
	return struct {
		Username string `yaml:"username"`
		Password any    `yaml:"password"`
	}{c.Username, password}, nil
}

func (c *PasswordCommand) validate() error {
	if c.Shell != "" && c.Args != nil {
		return errors.New("credentials.password.command must use either a shell string or arguments")
	}
	if c.Args != nil {
		if len(c.Args) == 0 || strings.TrimSpace(c.Args[0]) == "" {
			return errors.New("credentials.password.command requires an executable")
		}
		for _, arg := range c.Args {
			if strings.ContainsRune(arg, 0) {
				return errors.New("credentials.password.command arguments cannot contain NUL")
			}
		}
		return nil
	}
	if strings.TrimSpace(c.Shell) == "" || strings.ContainsRune(c.Shell, 0) {
		return errors.New("credentials.password.command requires a nonempty command without NUL")
	}
	return nil
}

// ValidatePassword checks the source without executing it.
func (c CredentialsConfig) ValidatePassword() error {
	if c.PasswordCommand != nil {
		if c.Password != "" {
			return errors.New("credentials.password cannot contain both a literal and a command")
		}
		return c.PasswordCommand.validate()
	}
	if c.Password == "" {
		return errors.New("missing credentials.password")
	}
	return nil
}

// ResolvePassword is only for live authentication. It never caches the result or
// mutates configuration. Errors deliberately omit commands, output, and exec errors.
func (c CredentialsConfig) ResolvePassword(ctx context.Context) (string, error) {
	if err := c.ValidatePassword(); err != nil {
		return "", err
	}
	if c.PasswordCommand == nil {
		return c.Password, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, passwordCommandTimeout)
	defer cancel()
	args := c.PasswordCommand.Args
	if args == nil {
		args = []string{"sh", "-c", c.PasswordCommand.Shell}
		if runtime.GOOS == "windows" {
			args = []string{"cmd.exe", "/C", c.PasswordCommand.Shell}
		}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	configurePasswordProcess(cmd)
	cmd.WaitDelay = time.Second
	var output passwordOutput
	cmd.Stdout = &output
	// Nil stdin/stderr use the null device: helpers cannot read terminal input or
	// print secrets into the TUI/logs. A password manager may use its own GUI agent.
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("password command canceled or timed out: %w", ctx.Err())
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("password command failed (exit code %d)", exit.ExitCode())
		}
		return "", errors.New("password command could not start or complete")
	}
	if output.overflow {
		return "", errors.New("password command output exceeds 64 KiB")
	}
	password := strings.TrimRight(output.buf.String(), "\r\n")
	if password == "" {
		return "", errors.New("password command returned an empty password")
	}
	if strings.ContainsAny(password, "\r\n\x00") {
		return "", errors.New("password command must return a single password line without NUL")
	}
	return password, nil
}

type passwordOutput struct {
	buf      bytes.Buffer
	overflow bool
}

func (b *passwordOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := maxPasswordOutput - b.buf.Len(); n > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return n, nil
}
