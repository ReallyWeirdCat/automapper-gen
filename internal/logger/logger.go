package logger

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

// LogLevel represents the verbosity level
type LogLevel int

const (
	LogLevelQuiet LogLevel = iota
	LogLevelNormal
	LogLevelVerbose
	LogLevelDebug
)

// ANSI color codes for terminal output
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorGray    = "\033[90m"
	ColorWhite   = "\033[97m"
)

// Logger handles all logging for the automapper generator
type Logger struct {
	level  LogLevel
	writer io.Writer
	colors bool // Whether to use colors
}

var defaultLogger *Logger

// init initializes the default logger
func init() {
	defaultLogger = &Logger{
		level:  LogLevelNormal,
		writer: os.Stdout,
		colors: true, // Will be properly detected on first use
	}
}

// SetLevel sets the global log level
func SetLevel(level LogLevel) {
	defaultLogger.level = level
}

// SetVerbose enables verbose logging
func SetVerbose(verbose bool) {
	if verbose {
		defaultLogger.level = LogLevelVerbose
	}
}

// SetColors enables or disables color output
func SetColors(enabled bool) {
	defaultLogger.colors = enabled
}

// detectColorSupport checks if the terminal supports colors
func detectColorSupport(writer io.Writer) bool {
	// Check for NO_COLOR environment variable (standard: https://no-color.org/)
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return false
	}
	
	// Check if output is a terminal
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	
	stat, err := file.Stat()
	if err != nil {
		return false
	}
	
	// Check if it's a character device (terminal)
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// ensureColorsDetected ensures color support is detected if needed
func (l *Logger) ensureColorsDetected() {
	// If colors haven't been explicitly set, detect them
	if l.colors {
		// Re-evaluate with actual detection
		l.colors = detectColorSupport(l.writer)
	}
}

// colorize wraps text with color if colors are enabled
func (l *Logger) colorize(text, color string) string {
	l.ensureColorsDetected()
	if l.colors {
		return color + text + ColorReset
	}
	return text
}

// Info logs informational messages (always shown unless quiet)
func Info(format string, args ...any) {
	if defaultLogger.level >= LogLevelNormal {
		prefix := "[INFO] "
		if defaultLogger.colors {
			prefix = defaultLogger.colorize("[INFO] ", ColorCyan)
		}
		fmt.Fprintf(defaultLogger.writer, prefix+format+"\n", args...)
	}
}

// Success logs success messages
func Success(format string, args ...any) {
	if defaultLogger.level >= LogLevelNormal {
		prefix := "[SUCCESS] "
		if defaultLogger.colors {
			prefix = defaultLogger.colorize("[SUCCESS] ", ColorGreen)
		}
		fmt.Fprintf(defaultLogger.writer, prefix+format+"\n", args...)
	}
}

// Warning logs warning messages
func Warning(format string, args ...any) {
	if defaultLogger.level >= LogLevelNormal {
		prefix := "[WARNING] "
		if defaultLogger.colors {
			prefix = defaultLogger.colorize("[WARNING] ", ColorYellow)
		}
		fmt.Fprintf(defaultLogger.writer, prefix+format+"\n", args...)
	}
}

// Error logs error messages (always shown)
func Error(format string, args ...any) {
	prefix := "[ERROR] "
	if defaultLogger.colors {
		prefix = defaultLogger.colorize("[ERROR] ", ColorRed)
	}
	fmt.Fprintf(os.Stderr, prefix+format+"\n", args...)
}

// Verbose logs detailed information (only in verbose mode)
func Verbose(format string, args ...any) {
	if defaultLogger.level >= LogLevelVerbose {
		prefix := "  [VERBOSE] "
		if defaultLogger.colors {
			prefix = defaultLogger.colorize("  [VERBOSE] ", ColorGray)
		}
		fmt.Fprintf(defaultLogger.writer, prefix+format+"\n", args...)
	}
}

// Debug logs debug information (only in debug mode)
func Debug(format string, args ...any) {
	if defaultLogger.level >= LogLevelDebug {
		// Include caller information for debug logs
		caller := ""
		if pc, file, line, ok := runtime.Caller(1); ok {
			funcName := runtime.FuncForPC(pc).Name()
			// Extract just the function name
			parts := strings.Split(funcName, ".")
			funcName = parts[len(parts)-1]
			caller = fmt.Sprintf("%s:%d %s", file, line, funcName)
		}
		
		prefix := "  [DEBUG] "
		if defaultLogger.colors {
			prefix = defaultLogger.colorize("  [DEBUG] ", ColorMagenta)
		}
		
		if caller != "" {
			callerInfo := ""
			if defaultLogger.colors {
				callerInfo = defaultLogger.colorize("("+caller+") ", ColorGray)
			} else {
				callerInfo = "(" + caller + ") "
			}
			fmt.Fprintf(defaultLogger.writer, prefix+callerInfo+format+"\n", args...)
		} else {
			fmt.Fprintf(defaultLogger.writer, prefix+format+"\n", args...)
		}
	}
}

// Section prints a section header
func Section(title string) {
	if defaultLogger.level >= LogLevelNormal {
		line := strings.Repeat("━", len(title)+4)
		if defaultLogger.colors {
			line = defaultLogger.colorize(line, ColorBlue)
			title = defaultLogger.colorize(title, ColorBlue)
		}
		fmt.Fprintf(defaultLogger.writer, "\n%s\n  %s  \n%s\n", line, title, line)
	}
}

// Step logs a step in the process
func Step(step int, total int, description string) {
	if defaultLogger.level >= LogLevelNormal {
		stepText := fmt.Sprintf("[%d/%d]", step, total)
		if defaultLogger.colors {
			stepText = defaultLogger.colorize(stepText, ColorCyan)
		}
		fmt.Fprintf(defaultLogger.writer, "%s %s\n", stepText, description)
	}
}

// Progress logs progress information with timing
func Progress(start time.Time, format string, args ...any) {
	if defaultLogger.level >= LogLevelVerbose {
		elapsed := time.Since(start)
		timeText := fmt.Sprintf("[%v]", elapsed.Round(time.Millisecond))
		if defaultLogger.colors {
			timeText = defaultLogger.colorize(timeText, ColorGray)
		}
		fmt.Fprintf(defaultLogger.writer, "  %s "+format+"\n", append([]any{timeText}, args...)...)
	}
}

// Stats logs statistics
func Stats(title string, stats map[string]any) {
	if defaultLogger.level >= LogLevelVerbose {
		titleText := title + ":"
		if defaultLogger.colors {
			titleText = defaultLogger.colorize(titleText, ColorCyan)
		}
		fmt.Fprintf(defaultLogger.writer, "\n%s\n", titleText)
		for k, v := range stats {
			key := fmt.Sprintf("  • %s: ", k)
			if defaultLogger.colors {
				key = defaultLogger.colorize(key, ColorWhite)
			}
			fmt.Fprintf(defaultLogger.writer, key+"%v\n", v)
		}
	}
}

// IsDebugEnabled returns true if debug logging is enabled
func IsDebugEnabled() bool {
	return defaultLogger.level >= LogLevelDebug
}

// IsVerboseEnabled returns true if verbose logging is enabled
func IsVerboseEnabled() bool {
	return defaultLogger.level >= LogLevelVerbose
}

// Fatal logs a fatal error and exits
func Fatal(format string, args ...any) {
	Error(format, args...)
	os.Exit(1)
}

// FatalErr logs a fatal error from an error object and exits
func FatalErr(err error) {
	Error("Fatal error: %v", err)
	os.Exit(1)
}
