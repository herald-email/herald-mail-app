package logger

import (
	"fmt"
	"io"
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
)

// Init initializes the logging system
func Init(debug bool) error {
	debugMode = debug
	
	// Create log file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("mail_processor_%s.log", timestamp)
	
	var err error
	logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Create loggers with different outputs
	var writers []io.Writer
	
	if debug {
		// In debug mode, write to both file and stdout
		writers = []io.Writer{logFile, os.Stdout}
	} else {
		// Normal mode, write only to file
		writers = []io.Writer{logFile}
	}
	
	multiWriter := io.MultiWriter(writers...)

	// Create different loggers for different levels
	infoLogger = log.New(multiWriter, "INFO  ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLogger = log.New(multiWriter, "ERROR ", log.Ldate|log.Ltime|log.Lshortfile)
	debugLogger = log.New(multiWriter, "DEBUG ", log.Ldate|log.Ltime|log.Lshortfile)

	Info("=== Mail Processor Started ===")
	Info("Logging to file: %s", filename)
	if debug {
		Info("Debug mode enabled - logs will also appear on console")
	}
	
	return nil
}

// Close closes the log file
func Close() {
	if logFile != nil {
		Info("=== Mail Processor Finished ===")
		logFile.Close()
	}
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	if infoLogger != nil {
		infoLogger.Printf(format, args...)
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	if errorLogger != nil {
		errorLogger.Printf(format, args...)
	}
}

// Debug logs a debug message (only if debug mode is enabled)
func Debug(format string, args ...interface{}) {
	if debugMode && debugLogger != nil {
		debugLogger.Printf(format, args...)
	}
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	if infoLogger != nil {
		infoLogger.Printf("WARN: "+format, args...)
	}
}

// IsDebugMode returns true if debug mode is enabled
func IsDebugMode() bool {
	return debugMode
}