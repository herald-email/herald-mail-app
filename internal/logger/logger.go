package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

var (
	infoLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
	logFile     *os.File
	debugMode   bool
	logCallback func(level, message string)
)

// Init initializes the logging system
func Init(debug bool) error {
	debugMode = debug
	
	// Create log file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("herald_%s.log", timestamp)
	
	var err error
	logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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
	Info("Logging to file: %s", filename)
	if debug {
		Info("Debug mode enabled - detailed logging active")
	}
	
	return nil
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

// Info logs an info message
func Info(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if infoLogger != nil {
		infoLogger.Print(message)
	}
	if logCallback != nil {
		logCallback("INFO", message)
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
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
		message := fmt.Sprintf(format, args...)
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
	message := fmt.Sprintf(format, args...)
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