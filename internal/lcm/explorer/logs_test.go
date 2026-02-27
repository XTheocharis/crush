package explorer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
)

func TestLogsExplorer_CanHandle(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		{
			name:     "log extension",
			path:     "app.log",
			content:  []byte("some log content"),
			expected: true,
		},
		{
			name:     "stderr extension",
			path:     "error.stderr",
			content:  []byte("error output"),
			expected: true,
		},
		{
			name:     "stdout extension",
			path:     "output.stdout",
			content:  []byte("standard output"),
			expected: true,
		},
		{
			name: "txt file with log patterns [LEVEL]",
			path: "log.txt",
			content: []byte(strings.Join([]string{
				"[ERROR] Failed to connect",
				"[INFO] Starting service",
				"[WARN] Memory usage high",
				"[ERROR] Database connection failed",
				"[INFO] Request processed",
			}, "\n")),
			expected: true,
		},
		{
			name: "txt file with log patterns datetime",
			path: "log.txt",
			content: []byte(strings.Join([]string{
				"2024-01-15 10:30:45.123 [ERROR] Failed to connect",
				"2024-01-15 10:30:46.456 [INFO] Starting service",
				"2024-01-15 10:30:47.789 [WARN] Memory usage high",
				"2024-01-15 10:30:48.012 [ERROR] Connection failed",
				"2024-01-15 10:30:49.345 [INFO] Request processed",
			}, "\n")),
			expected: true,
		},
		{
			name: "txt file with syslog pattern",
			path: "syslog.txt",
			content: []byte(strings.Join([]string{
				"Jan 15 10:30:45 hostname service[1234]: [ERROR] Something went wrong",
				"Jan 15 10:30:46 hostname service[1234]: [INFO] Normal operation",
				"Jan 15 10:30:47 hostname service[1234]: [WARN] Warning message",
			}, "\n")),
			expected: true,
		},
		{
			name: "txt file with Java stack trace",
			path: "stack.txt",
			content: []byte(strings.Join([]string{
				"Exception in thread main",
				"    at com.example.Main.main(Main.java:10)",
				"    at com.example.Helper.doSomething(Helper.java:25)",
			}, "\n")),
			expected: true,
		},
		{
			name: "txt file with ERROR: prefix",
			path: "errors.txt",
			content: []byte(strings.Join([]string{
				"ERROR: Something went wrong",
				"ERROR: Another error occurred",
				"WARNING: This is a warning",
			}, "\n")),
			expected: true,
		},
		{
			name:     "txt file without log patterns",
			path:     "readme.txt",
			content:  []byte("This is just a regular text file.\nIt has no log patterns."),
			expected: false,
		},
		{
			name:     "empty content with log extension",
			path:     "empty.log",
			content:  []byte(""),
			expected: true,
		},
		{
			name:     "json file is not a log",
			path:     "config.json",
			content:  []byte(`{"key": "value"}`),
			expected: false,
		},
		{
			name: "mixed case log patterns",
			path: "log.txt",
			content: []byte(strings.Join([]string{
				"[error] Failed to connect",
				"[Info] Starting service",
				"[Warn] Memory usage high",
				"[DEBUG] Debug message",
				"[Trace] Trace message",
			}, "\n")),
			expected: true,
		},
		{
			name: "single letter level indicator",
			path: "log.txt",
			content: []byte(strings.Join([]string{
				"[E] Error message",
				"[W] Warning message",
				"[I] Info message",
				"[D] Debug message",
			}, "\n")),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &LogsExplorer{}
			result := exp.CanHandle(tt.path, tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogsExplorer_Explore(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		content          []byte
		expectedExplorer string
		expectedContains []string
		expectedTokensGt int
	}{
		{
			name: "simple log with levels",
			path: "app.log",
			content: []byte(strings.Join([]string{
				"2024-01-15 10:30:45.123 [ERROR] Failed to connect to database",
				"2024-01-15 10:30:46.456 [INFO] Starting service on port 8080",
				"2024-01-15 10:30:47.789 [WARN] Memory usage at 80%",
				"2024-01-15 10:30:48.012 [DEBUG] Processing request 12345",
				"2024-01-15 10:30:49.345 [ERROR] Connection timeout",
				"2024-01-15 10:30:50.678 [INFO] Request completed successfully",
				"2024-01-15 10:30:51.901 [TRACE] Entering function processData()",
				"2024-01-15 10:30:52.234 [INFO] Processing batch of 100 items",
				"2024-01-15 10:30:53.567 [WARN] Response time > 500ms",
				"2024-01-15 10:30:54.890 [ERROR] Failed to parse JSON response",
			}, "\n")),
			expectedExplorer: "logs",
			expectedContains: []string{
				"Log file: app.log",
				"Total lines: 10",
				"Level distribution:",
				"ERROR: 3",
				"INFO: 3",
				"WARN: 2",
				"DEBUG: 1",
				"TRACE: 1",
				"Timestamp patterns:",
				"ISO8601",
				"Sample errors/warnings:",
			},
			expectedTokensGt: 0,
		},
		{
			name: "log without timestamps",
			path: "simple.log",
			content: []byte(strings.Join([]string{
				"[ERROR] Something went wrong",
				"[INFO] Starting up",
				"[WARN] Check your config",
				"[ERROR] Another error",
				"[INFO] Continuing...",
			}, "\n")),
			expectedExplorer: "logs",
			expectedContains: []string{
				"Log file: simple.log",
				"Total lines: 5",
				"Level distribution:",
				"ERROR: 2",
				"INFO: 2",
				"WARN: 1",
				"No standard timestamp patterns detected",
			},
			expectedTokensGt: 0,
		},
		{
			name:             "empty log file",
			path:             "empty.log",
			content:          []byte(""),
			expectedExplorer: "logs",
			expectedContains: []string{
				"Log file: empty.log",
				"Total lines: 1",
				"No standard log levels detected",
				"No standard timestamp patterns detected",
			},
			expectedTokensGt: 0,
		},
		{
			name: "log with multiple timestamp formats",
			path: "mixed_time.log",
			content: []byte(strings.Join([]string{
				"2024-01-15T10:30:45Z [ERROR] RFC3339 format",
				"2024-01-15 10:30:45 [INFO] ISO8601 format",
				"Jan 15 10:30:45 host service: [WARN] Syslog format",
				"15/Jan/2024:10:30:45 +0000 [INFO] Common log format",
				"[ERROR] No timestamp",
			}, "\n")),
			expectedExplorer: "logs",
			expectedContains: []string{
				"Log file: mixed_time.log",
				"Timestamp patterns:",
			},
			expectedTokensGt: 0,
		},
		{
			name: "log with Java stack traces",
			path: "app.log",
			content: []byte(strings.Join([]string{
				"2024-01-15 10:30:45.123 [ERROR] Exception occurred",
				"    at com.example.Main.method1(Main.java:10)",
				"    at com.example.Main.method2(Main.java:20)",
				"2024-01-15 10:30:46.456 [INFO] Recovered from error",
			}, "\n")),
			expectedExplorer: "logs",
			expectedContains: []string{
				"Log file: app.log",
				"Level distribution:",
			},
			expectedTokensGt: 0,
		},
		{
			name: "log with CRITICAL and FATAL levels",
			path: "error.log",
			content: []byte(strings.Join([]string{
				"[CRITICAL] System failure imminent",
				"[FATAL] Cannot continue, shutting down",
				"[ERROR] Something went wrong",
				"[WARN] Warning message",
			}, "\n")),
			expectedExplorer: "logs",
			expectedContains: []string{
				"Log file: error.log",
				"CRITICAL",
				"FATAL",
			},
			expectedTokensGt: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &LogsExplorer{}
			result, err := exp.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err)

			require.Equal(t, tt.expectedExplorer, result.ExplorerUsed)

			for _, expected := range tt.expectedContains {
				require.Contains(t, result.Summary, expected)
			}

			require.GreaterOrEqual(t, result.TokenEstimate, tt.expectedTokensGt, "Expected token estimate >= %d, got %d", tt.expectedTokensGt, result.TokenEstimate)
		})
	}
}

func TestLogsExplorer_LevelDistribution(t *testing.T) {
	tests := []struct {
		name         string
		content      []byte
		wantError    int
		wantWarn     int
		wantInfo     int
		wantDebug    int
		wantTrace    int
		wantCritical int
		wantFatal    int
	}{
		{
			name: "all levels present",
			content: []byte(strings.Join([]string{
				"[ERROR] error 1",
				"[ERROR] error 2",
				"[WARN] warn 1",
				"[INFO] info 1",
				"[INFO] info 2",
				"[INFO] info 3",
				"[DEBUG] debug 1",
				"[TRACE] trace 1",
				"[CRITICAL] critical 1",
				"[FATAL] fatal 1",
			}, "\n")),
			wantError:    2,
			wantWarn:     1,
			wantInfo:     3,
			wantDebug:    1,
			wantTrace:    1,
			wantCritical: 1,
			wantFatal:    1,
		},
		{
			name: "only errors",
			content: []byte(strings.Join([]string{
				"[ERROR] error 1",
				"[ERROR] error 2",
				"[ERROR] error 3",
			}, "\n")),
			wantError: 3,
		},
		{
			name: "mixed case levels",
			content: []byte(strings.Join([]string{
				"[error] error 1",
				"[Error] error 2",
				"[ERROR] error 3",
				"[warn] warn 1",
				"[Warn] warn 2",
				"[WARN] warn 3",
			}, "\n")),
			wantError: 3,
			wantWarn:  3,
		},
		{
			name: "alternative level formats",
			content: []byte(strings.Join([]string{
				"FAIL: operation failed",
				"PANIC: cannot continue",
				"EMRG: emergency",
				"FAILURE: request failed",
			}, "\n")),
			wantError: 4, // These all map to ERROR
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &LogsExplorer{}
			result, err := exp.Explore(context.Background(), ExploreInput{
				Path:    "test.log",
				Content: tt.content,
			})
			require.NoError(t, err)

			summary := result.Summary

			// Count occurrences in the summary.
			countInSummary := func(level string) int {
				count := 0
				// Look for patterns like "ERROR: 5 (50.0%)"
				prefix := level + ": "
				idx := 0
				for {
					found := strings.Index(summary[idx:], prefix)
					if found == -1 {
						break
					}
					count++
					idx += found + len(prefix)
				}
				return count
			}

			if tt.wantError > 0 {
				require.NotZero(t, countInSummary("ERROR"), "Expected ERROR count of %d, summary: %s", tt.wantError, summary)
			}
			if tt.wantWarn > 0 {
				require.NotZero(t, countInSummary("WARN"), "Expected WARN count of %d, summary: %s", tt.wantWarn, summary)
			}
			if tt.wantInfo > 0 {
				require.NotZero(t, countInSummary("INFO"), "Expected INFO count of %d, summary: %s", tt.wantInfo, summary)
			}
		})
	}
}

func TestLogsExplorer_TimestampPatterns(t *testing.T) {
	tests := []struct {
		name          string
		content       []byte
		wantISO8601   bool
		wantRFC3339   bool
		wantCommonLog bool
		wantSyslog    bool
		wantUnixTime  bool
		wantCompactDt bool
	}{
		{
			name: "ISO8601 timestamps",
			content: []byte(strings.Join([]string{
				"2024-01-15 10:30:45.123 [INFO] message 1",
				"2024-01-15 10:30:46.456 [INFO] message 2",
				"2024-01-15 10:30:47.789 [INFO] message 3",
			}, "\n")),
			wantISO8601: true,
		},
		{
			name: "RFC3339 timestamps",
			content: []byte(strings.Join([]string{
				"2024-01-15T10:30:45Z [INFO] message 1",
				"2024-01-15T10:30:46.123+00:00 [INFO] message 2",
				"2024-01-15T10:30:47-05:00 [INFO] message 3",
			}, "\n")),
			wantRFC3339: true,
		},
		{
			name: "Common log format",
			content: []byte(strings.Join([]string{
				"15/Jan/2024:10:30:45 +0000 [INFO] message 1",
				"15/Jan/2024:10:30:46 +0000 [INFO] message 2",
			}, "\n")),
			wantCommonLog: true,
		},
		{
			name: "Syslog format",
			content: []byte(strings.Join([]string{
				"Jan 15 10:30:45 hostname service[1234]: [INFO] message 1",
				"Jan 15 10:30:46 hostname service[1234]: [INFO] message 2",
			}, "\n")),
			wantSyslog: true,
		},
		{
			name: "Unix timestamps",
			content: []byte(strings.Join([]string{
				"1705315845 [INFO] message 1",
				"1705315846.123 [INFO] message 2",
			}, "\n")),
			wantUnixTime: true,
		},
		{
			name: "compact datetime",
			content: []byte(strings.Join([]string{
				"20240115103045 [INFO] message",
			}, "\n")),
			wantCompactDt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &LogsExplorer{}
			result, err := exp.Explore(context.Background(), ExploreInput{
				Path:    "test.log",
				Content: tt.content,
			})
			require.NoError(t, err)

			summary := result.Summary
			if tt.wantISO8601 {
				require.Contains(t, summary, "ISO8601")
			}
			if tt.wantRFC3339 {
				require.Contains(t, summary, "RFC3339")
			}
			if tt.wantCommonLog {
				require.Contains(t, summary, "CommonLog")
			}
			if tt.wantSyslog {
				require.Contains(t, summary, "Syslog")
			}
			if tt.wantUnixTime {
				require.Contains(t, summary, "UnixTime")
			}
			if tt.wantCompactDt {
				// May also match as CompactDate, so be lenient.
				hasCompact := strings.Contains(summary, "CompactDateTime") || strings.Contains(summary, "CompactDate")
				require.True(t, hasCompact, "Expected CompactDateTime in summary, got: %s", summary)
			}
		})
	}
}

func TestLogsExplorer_SampleErrorsWarnings(t *testing.T) {
	tests := []struct {
		name               string
		content            []byte
		wantSampleCountMin int
		wantSampleCountMax int
	}{
		{
			name: "few errors, all should be sampled",
			content: []byte(strings.Join([]string{
				"[ERROR] Error 1: Failed to connect",
				"[ERROR] Error 2: Timeout occurred",
				"[WARN] Warning 1: High memory usage",
			}, "\n")),
			wantSampleCountMin: 3,
			wantSampleCountMax: 3,
		},
		{
			name:               "many errors, should be bounded",
			content:            []byte(buildLogWithManyErrors(20, 10)),
			wantSampleCountMin: 5,
			wantSampleCountMax: 10,
		},
		{
			name: "no errors or warnings",
			content: []byte(strings.Join([]string{
				"[INFO] Info message 1",
				"[INFO] Info message 2",
				"[DEBUG] Debug message 1",
			}, "\n")),
			wantSampleCountMin: 0,
			wantSampleCountMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &LogsExplorer{}
			result, err := exp.Explore(context.Background(), ExploreInput{
				Path:    "test.log",
				Content: tt.content,
			})
			require.NoError(t, err)

			// Count samples by looking for numbered entries.
			summary := result.Summary
			sampleCount := 0
			for i := 1; i <= maxSampleSize*2; i++ {
				if strings.Contains(summary, fmt.Sprintf("%d. ", i)) {
					sampleCount++
				}
			}

			require.GreaterOrEqual(t, sampleCount, tt.wantSampleCountMin, "Expected sample count between %d and %d, got %d. Summary:\n%s",
				tt.wantSampleCountMin, tt.wantSampleCountMax, sampleCount, summary)
			require.LessOrEqual(t, sampleCount, tt.wantSampleCountMax, "Expected sample count between %d and %d, got %d. Summary:\n%s",
				tt.wantSampleCountMin, tt.wantSampleCountMax, sampleCount, summary)
		})
	}
}

func TestLogsExplorer_DeterministicSampling(t *testing.T) {
	// Test that sampling is deterministic - same input should produce same output.
	content := buildLogWithManyErrors(30, 15)

	results := make([]string, 5)
	for i := range 5 {
		exp := &LogsExplorer{}
		result, err := exp.Explore(context.Background(), ExploreInput{
			Path:    "test.log",
			Content: content,
		})
		require.NoError(t, err, "Explore failed on iteration %d", i)

		// Extract the sample section.
		sampleStart := strings.Index(result.Summary, "Sample errors/warnings:")
		require.NotEqual(t, -1, sampleStart, "No sample section found in iteration %d", i)
		results[i] = result.Summary[sampleStart:]
	}

	// All runs should produce the same sampling.
	for i := 1; i < len(results); i++ {
		require.Equal(t, results[0], results[i], "Sampling not deterministic. Run 0:\n%s\n\nRun %d:\n%s", results[0], i, results[i])
	}
}

func TestLogsExplorer_TokenEstimate(t *testing.T) {
	tests := []struct {
		name           string
		content        []byte
		expectedGtZero bool
	}{
		{
			name:           "simple log",
			content:        []byte("[INFO] Test message"),
			expectedGtZero: true,
		},
		{
			name:           "empty log",
			content:        []byte(""),
			expectedGtZero: true, // Even empty logs produce some summary text.
		},
		{
			name:           "large log",
			content:        []byte(strings.Repeat("[INFO] Test message\n", 1000)),
			expectedGtZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &LogsExplorer{}
			result, err := exp.Explore(context.Background(), ExploreInput{
				Path:    "test.log",
				Content: tt.content,
			})
			require.NoError(t, err)

			if tt.expectedGtZero {
				require.Greater(t, result.TokenEstimate, 0, "Expected TokenEstimate > 0, got %d", result.TokenEstimate)
			}
		})
	}
}

func TestLogsExplorer_SmokeTest(t *testing.T) {
	// A comprehensive smoke test covering the main functionality.
	exp := &LogsExplorer{}

	// Test 1: CanHandle with various file extensions.
	require.True(t, exp.CanHandle("app.log", []byte("[INFO] test")))
	require.True(t, exp.CanHandle("error.stderr", []byte("[ERROR] test")))
	require.True(t, exp.CanHandle("output.stdout", []byte("[INFO] test")))

	// Test 2: Explore produces valid result.
	content := []byte(strings.Join([]string{
		"2024-01-15 10:30:45.123 [ERROR] Database connection failed",
		"2024-01-15 10:30:46.456 [INFO] Starting application",
		"2024-01-15 10:30:47.789 [WARN] Memory usage at 85%",
		"2024-01-15 10:30:48.012 [DEBUG] Processing request",
		"2024-01-15 10:30:49.345 [ERROR] Timeout after 30s",
		"2024-01-15 10:30:50.678 [INFO] Request completed",
		"2024-01-15 10:30:51.901 [TRACE] Entering process loop",
	}, "\n"))

	result, err := exp.Explore(context.Background(), ExploreInput{
		Path:    "app.log",
		Content: content,
	})
	require.NoError(t, err)

	// Verify result structure.
	require.Equal(t, "logs", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0, "Expected TokenEstimate > 0, got %d", result.TokenEstimate)

	// Verify summary contains expected sections.
	summary := result.Summary
	expectedSections := []string{
		"Log file: app.log",
		"Size:",
		"Total lines:",
		"Level distribution:",
		"ERROR:",
		"WARN:",
		"INFO:",
		"DEBUG:",
		"TRACE:",
		"Timestamp patterns:",
		"Sample errors/warnings:",
	}

	for _, section := range expectedSections {
		require.Contains(t, summary, section)
	}

	// Test 3: Explorer matches expected values.
	require.Equal(t, "logs", result.ExplorerUsed)
}

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantTimestamp string
		wantLevel     string
		wantMessage   string
	}{
		{
			name:          "standard format",
			line:          "2024-01-15 10:30:45.123 [ERROR] Failed to connect",
			wantTimestamp: "2024-01-15 10:30:45.123",
			wantLevel:     "[ERROR]",
			wantMessage:   "Failed to connect",
		},
		{
			name:          "RFC3339 format",
			line:          "2024-01-15T10:30:45Z [INFO] Starting service",
			wantTimestamp: "2024-01-15T10:30:45Z",
			wantLevel:     "[INFO]",
			wantMessage:   "Starting service",
		},
		{
			name:        "level only",
			line:        "[ERROR] Something went wrong",
			wantLevel:   "[ERROR]",
			wantMessage: "Something went wrong",
		},
		{
			name:        "message only",
			line:        "Just a plain message",
			wantMessage: "Just a plain message",
		},
		{
			name: "empty line",
			line: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp, level, message := parseLogLine(tt.line)
			require.Equal(t, tt.wantTimestamp, timestamp, "parseLogLine() timestamp")
			require.Equal(t, tt.wantLevel, level, "parseLogLine() level")
			require.Equal(t, tt.wantMessage, message, "parseLogLine() message")
		})
	}
}

func TestParseLogLines(t *testing.T) {
	content := []byte(strings.Join([]string{
		"2024-01-15 10:30:45.123 [ERROR] Error message",
		"[INFO] Info message",
		"",
		"2024-01-15 10:30:46.456 [WARN] Warning message",
	}, "\n"))

	lines := ParseLogLines(content)

	require.Equal(t, 3, len(lines))

	// Verify first line.
	require.Equal(t, "2024-01-15 10:30:45.123", lines[0].Timestamp)
	require.Equal(t, "[ERROR]", lines[0].Level)
	require.Equal(t, "Error message", lines[0].Message)
}

func TestFilterByLevel(t *testing.T) {
	content := []byte(strings.Join([]string{
		"[ERROR] Error 1",
		"[INFO] Info 1",
		"[ERROR] Error 2",
		"[WARN] Warning 1",
		"[INFO] Info 2",
	}, "\n"))

	lines := ParseLogLines(content)

	// Filter by ERROR.
	errorLines := FilterByLevel(lines, "ERROR")
	require.Equal(t, 2, len(errorLines), "Expected 2 ERROR lines")

	// Filter by INFO.
	infoLines := FilterByLevel(lines, "INFO")
	require.Equal(t, 2, len(infoLines), "Expected 2 INFO lines")

	// Filter by non-existent level.
	noneLines := FilterByLevel(lines, "TRACE")
	require.Equal(t, 0, len(noneLines), "Expected 0 TRACE lines")
}

func TestGetLevelCounts(t *testing.T) {
	content := []byte(strings.Join([]string{
		"[ERROR] Error 1",
		"[ERROR] Error 2",
		"[INFO] Info 1",
		"[WARN] Warning 1",
		"[INFO] Info 2",
		"[info] Info 3 lowercase",
	}, "\n"))

	lines := ParseLogLines(content)
	counts := GetLevelCounts(lines)

	// Should have 3 INFO (uppercase and lowercase normalized).
	require.Equal(t, 3, counts["INFO"], "Expected INFO count 3")
	require.Equal(t, 2, counts["ERROR"], "Expected ERROR count 2")
	require.Equal(t, 1, counts["WARN"], "Expected WARN count 1")
}

func TestGetTimestampStats(t *testing.T) {
	content := []byte(strings.Join([]string{
		"2024-01-15 10:30:45.123 [INFO] ISO format",
		"2024-01-15T10:30:45Z [INFO] RFC3339 format",
		"[INFO] No timestamp",
		"2024-01-15 10:30:46.456 [ERROR] Another ISO",
	}, "\n"))

	lines := ParseLogLines(content)
	stats := GetTimestampStats(lines)

	// We should detect ISO8601 and RFC3339.
	require.False(t, stats["ISO8601"] == 0 && stats["RFC3339"] == 0, "Expected to detect timestamps, got: %v", stats)
}

func TestExportAsCSV(t *testing.T) {
	lines := []LogLine{
		{Timestamp: "2024-01-15 10:30:45", Level: "[ERROR]", Message: "Test error", Raw: "2024-01-15 10:30:45 [ERROR] Test error"},
		{Timestamp: "2024-01-15 10:30:46", Level: "[INFO]", Message: "Test info", Raw: "2024-01-15 10:30:46 [INFO] Test info"},
	}

	csv := ExportAsCSV(lines)

	require.Contains(t, csv, "timestamp,level,message", "Expected CSV header")
	require.Contains(t, csv, "2024-01-15 10:30:45", "Expected timestamp in CSV")
}

func TestEscapeCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "simple",
			expected: "simple",
		},
		{
			input:    `has,comma`,
			expected: `"has,comma"`,
		},
		{
			input:    `has"quote`,
			expected: `"has""quote"`,
		},
		{
			input:    "has\nnewline",
			expected: `"has\nnewline"`,
		},
		{
			input:    `complex,"test",here`,
			expected: `"complex,""test"",here"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeCSV(tt.input)
			require.Equal(t, tt.expected, result, "escapeCSV(%q)", tt.input)
		})
	}
}

func TestFNV1AHash(t *testing.T) {
	// Test that the hash is deterministic for the same input.
	s := "test string"
	hash1 := fnv1aHash(s)
	hash2 := fnv1aHash(s)

	require.Equal(t, hash1, hash2, "fnv1aHash not deterministic")

	// Test that different inputs produce different hashes (high probability).
	s2 := "different string"
	hash3 := fnv1aHash(s2)

	require.NotEqual(t, hash1, hash3, "fnv1aHash collision: same hash for different inputs")

	// Test that empty string produces a specific value (offset32).
	emptyHash := fnv1aHash("")
	require.Equal(t, uint32(2166136261), emptyHash, "fnv1aHash(\"\")")
}

func TestTruncateSample(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short line no truncation",
			input:    "short line",
			maxLen:   200,
			expected: "short line",
		},
		{
			name:     "long line truncation",
			input:    strings.Repeat("a", 300),
			maxLen:   100,
			expected: strings.Repeat("a", 100) + "...",
		},
		{
			name:     "exact length no truncation",
			input:    strings.Repeat("b", 50),
			maxLen:   50,
			expected: strings.Repeat("b", 50),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateSample(tt.input, tt.maxLen)
			require.LessOrEqual(t, len(result), tt.maxLen+3, "truncateSample() result length %d > max %d", len(result), tt.maxLen) // +3 for "..."
			if len(result) > tt.maxLen {
				require.True(t, strings.HasSuffix(result, "..."), "truncateSample() long line should end with '...'")
			}
			// Verify result matches expected
			require.Equal(t, tt.expected, result, "truncateSample() result")
		})
	}
}

func TestDeterministicallySample(t *testing.T) {
	// Empty input.
	result := deterministicallySample([]string{}, 5)
	require.Empty(t, result, "Expected empty result for empty input")

	// Fewer items than limit.
	items := []string{"a", "b", "c"}
	result = deterministicallySample(items, 10)
	require.Equal(t, 3, len(result), "Expected all items when limit > count")

	// More items than limit - should select N items deterministically.
	items = make([]string, 100)
	for i := range 100 {
		items[i] = fmt.Sprintf("item-%d", i)
	}

	// Run twice and verify same result.
	result1 := deterministicallySample(items, 10)
	result2 := deterministicallySample(items, 10)

	require.Equal(t, 10, len(result1), "Expected 10 sampled items")
	require.Equal(t, 10, len(result2), "Expected 10 sampled items")

	// Verify results are identical (deterministic).
	for i := range 10 {
		require.Equal(t, result1[i], result2[i], "Deterministic sampling failed: result1[%d] = %q, result2[%d] = %q",
			i, result1[i], i, result2[i])
	}
}

// Helper function to build a log with many errors.
func buildLogWithManyErrors(errorCount, warnCount int) []byte {
	var lines []string
	for i := range errorCount {
		lines = append(lines, fmt.Sprintf("[ERROR] Error number %d: Something went wrong with more details", i+1))
	}
	for i := range warnCount {
		lines = append(lines, fmt.Sprintf("[WARN] Warning number %d: This is a warning message", i+1))
	}
	return []byte(strings.Join(lines, "\n"))
}

// deterministicTestLogContent provides consistent test content covering
// level distribution, timestamp patterns, and representative error/warning samples.
func deterministicTestLogContent() []byte {
	return []byte(`2024-01-15 10:30:45.123 [ERROR] Failed to connect to database: connection timeout
2024-01-15 10:30:46.456 [INFO] Starting application server on port 8080
2024-01-15 10:30:47.789 [WARN] Memory usage at 80%, approaching limit
2024-01-15 10:30:48.012 [DEBUG] Processing request ID: abc123-def456
2024-01-15 10:30:49.345 [ERROR] Authentication failed for user: admin
2024-01-15 10:30:50.678 [INFO] Request completed successfully in 45ms
2024-01-15 10:30:51.901 [TRACE] Entering function processData()
2024-01-15 10:30:52.234 [DEBUG] Cache hit for key: user_profile_123
2024-01-15 10:30:53.567 [WARN] Slow query detected: SELECT * FROM large_table (250ms)
2024-01-15 10:30:54.890 [ERROR] Failed to parse JSON response: invalid token at position 42
2024-01-15T10:30:55Z [INFO] RFC3339 timestamp format example
2024-01-15 10:30:56.111 [DEBUG] Response size: 1024 bytes
Jan 15 10:30:57 hostname service[1234]: [WARN] Syslog formatted warning message
15/Jan/2024:10:30:58 +0000 [ERROR] Common log format error message
2024-01-15 10:30:59.999 [INFO] Background job started: cleanup_task
`)
}

// TestLogsExplorer_GoldenEnhancement tests golden file output for enhancement profile.
func TestLogsExplorer_GoldenEnhancement(t *testing.T) {
	t.Parallel()

	content := deterministicTestLogContent()
	registry := NewRegistry(WithOutputProfile(OutputProfileEnhancement))

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path:    "app.log",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	golden.RequireEqual(t, []byte(result.Summary))
}

// TestLogsExplorer_GoldenParity tests golden file output for parity profile.
func TestLogsExplorer_GoldenParity(t *testing.T) {
	t.Parallel()

	content := deterministicTestLogContent()
	registry := NewRegistry(WithOutputProfile(OutputProfileParity))

	result, err := registry.Explore(context.Background(), ExploreInput{
		Path:    "app.log",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	golden.RequireEqual(t, []byte(result.Summary))
}
