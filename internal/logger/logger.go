package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

const privateMask = "?????????"

var (
	infoLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
	logFile     *os.File
	logPath     string
	debugMode   bool
	unsafeLogs  bool
	logCallback func(level, message string)

	emailPattern             = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-']+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	homePathPattern          = regexp.MustCompile(`(?i)(?:/Users|/home|/private/var/folders|/var/folders)/[^\s"')]+`)
	sensitiveLinePattern     = regexp.MustCompile(`(?im)^(\s*(?:folder|local[_ -]?id|message[_ -]?id|sender|subject)\s*:\s*).+$`)
	sensitiveKeyValuePattern = regexp.MustCompile(`(?i)\b(email|username|user|sender|from|to|cc|bcc|reply[_ -]?to|subject|folder|mailbox|label|message[_ -]?id|local[_ -]?id|config[_ -]?path|log[_ -]?file|cache[_ -]?(?:database|path)|path|password|passwd|secret|token|access[_ -]?token|refresh[_ -]?token|authorization|auth)(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|<[^>]*>|[^\s,;]+)`)
)

type options struct {
	unsafeLogs bool
}

// Option customizes logger initialization.
type Option func(*options)

// WithUnsafeLogs disables privacy masking for log output. This should only be
// used for explicit local diagnostics because private mailbox data may appear.
func WithUnsafeLogs(enabled bool) Option {
	return func(o *options) {
		o.unsafeLogs = enabled
	}
}

// Init initializes the logging system
func Init(debug bool, opts ...Option) error {
	settings := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&settings)
		}
	}
	debugMode = debug
	unsafeLogs = settings.unsafeLogs

	// Create log file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("herald_%s.log", timestamp)

	logDir, err := defaultLogDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	logPath = filepath.Join(logDir, filename)

	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Always write only to file (no console output to keep TUI clean)
	multiWriter := logFile

	// Create different loggers for different levels
	infoLogger = log.New(multiWriter, "INFO  ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(multiWriter, "ERROR ", log.Ldate|log.Ltime|log.Lshortfile)
	debugLogger = log.New(multiWriter, "DEBUG ", log.Ldate|log.Ltime|log.Lshortfile)

	Info("=== Herald Started ===")
	Info("Logging to file: %s", logPath)
	if debug {
		Info("Debug mode enabled - detailed logging active")
	}
	if unsafeLogs {
		Warn("Unsafe logs enabled - private mailbox data may be written unredacted")
	}

	return nil
}

// Path returns the active log file path, when logging has been initialized.
func Path() string {
	return logPath
}

func defaultLogDir() (string, error) {
	if override := os.Getenv("HERALD_LOG_DIR"); override != "" {
		return override, nil
	}

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to resolve home directory for logs: %w", err)
		}
		return filepath.Join(home, "Library", "Logs", "Herald"), nil
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "Herald", "Logs"), nil
		}
	default:
		if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
			return filepath.Join(stateHome, "herald", "logs"), nil
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "state", "herald", "logs"), nil
		}
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user log directory: %w", err)
	}
	return filepath.Join(cacheDir, "herald", "logs"), nil
}

// Close closes the log file
func Close() {
	if logFile != nil {
		Info("=== Herald Finished ===")
		logFile.Close()
	}
}

// SetLogCallback sets a callback function to receive log messages
func SetLogCallback(callback func(level, message string)) {
	logCallback = callback
}

// Redact masks private mailbox and local-account data in a string.
func Redact(message string) string {
	message = sensitiveLinePattern.ReplaceAllString(message, "${1}"+privateMask)
	message = maskSensitiveKeyValues(message)
	message = emailPattern.ReplaceAllString(message, privateMask)
	message = homePathPattern.ReplaceAllString(message, privateMask)
	return message
}

func safeMessage(message string) string {
	if unsafeLogs {
		return message
	}
	return Redact(message)
}

func maskSensitiveKeyValues(message string) string {
	return sensitiveKeyValuePattern.ReplaceAllStringFunc(message, func(match string) string {
		parts := sensitiveKeyValuePattern.FindStringSubmatchIndex(match)
		if len(parts) < 6 || parts[2] < 0 || parts[4] < 0 {
			return privateMask
		}
		return match[parts[2]:parts[3]] + match[parts[4]:parts[5]] + privateMask
	})
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	message := safeMessage(fmt.Sprintf(format, args...))
	if infoLogger != nil {
		infoLogger.Print(message)
	}
	if logCallback != nil {
		logCallback("INFO", message)
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	message := safeMessage(fmt.Sprintf(format, args...))
	if errorLogger != nil {
		errorLogger.Print(message)
	}
	if logCallback != nil {
		logCallback("ERROR", message)
	}
}

// Debug logs a debug message (only if debug mode is enabled)
func Debug(format string, args ...interface{}) {
	if debugMode {
		message := safeMessage(fmt.Sprintf(format, args...))
		if debugLogger != nil {
			debugLogger.Print(message)
		}
		if logCallback != nil {
			logCallback("DEBUG", message)
		}
	}
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	message := safeMessage(fmt.Sprintf(format, args...))
	if infoLogger != nil {
		infoLogger.Print("WARN: " + message)
	}
	if logCallback != nil {
		logCallback("WARN", message)
	}
}

// IsDebugMode returns true if debug mode is enabled
func IsDebugMode() bool {
	return debugMode
}

// Logger is an injectable logger that delegates to the package-level logger.
// A nil *Logger is safe to use; all methods are no-ops on nil.
type Logger struct{}

// New returns a Logger that delegates to the package-level logger functions.
func New() *Logger { return &Logger{} }

// Debug logs a debug message via the package-level Debug function.
func (l *Logger) Debug(format string, args ...interface{}) {
	if l == nil {
		return
	}
	Debug(format, args...)
}

// Info logs an info message via the package-level Info function.
func (l *Logger) Info(format string, args ...interface{}) {
	if l == nil {
		return
	}
	Info(format, args...)
}
