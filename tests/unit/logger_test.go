package unit

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/kmlixh/consulTool/logger"
)

var levelNames = map[logger.Level]string{
	logger.DEBUG: "DEBUG",
	logger.INFO:  "INFO",
	logger.WARN:  "WARN",
	logger.ERROR: "ERROR",
	logger.FATAL: "FATAL",
}

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewLogger(&logger.Options{
		Level:  logger.DEBUG,
		Output: &buf,
	})

	tests := []struct {
		level    logger.Level
		logFunc  func(...interface{})
		logFuncf func(string, ...interface{})
		message  string
		format   string
		args     []interface{}
	}{
		{logger.DEBUG, log.Debug, log.Debugf, "debug message", "debug %s", []interface{}{"message"}},
		{logger.INFO, log.Info, log.Infof, "info message", "info %s", []interface{}{"message"}},
		{logger.WARN, log.Warn, log.Warnf, "warn message", "warn %s", []interface{}{"message"}},
		{logger.ERROR, log.Error, log.Errorf, "error message", "error %s", []interface{}{"message"}},
	}

	for _, tt := range tests {
		buf.Reset()
		tt.logFunc(tt.message)
		output := buf.String()

		if !strings.Contains(output, levelNames[tt.level]) {
			t.Errorf("Expected log level %v in output, got: %s", tt.level, output)
		}
		if !strings.Contains(output, tt.message) {
			t.Errorf("Expected message %q in output, got: %s", tt.message, output)
		}

		buf.Reset()
		tt.logFuncf(tt.format, tt.args...)
		output = buf.String()

		expectedMsg := strings.Replace(tt.format, "%s", tt.args[0].(string), -1)
		if !strings.Contains(output, expectedMsg) {
			t.Errorf("Expected formatted message %q in output, got: %s", expectedMsg, output)
		}
	}
}

func TestLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewLogger(&logger.Options{
		Level:  logger.INFO,
		Output: &buf,
	})

	fields := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	log = log.WithFields(fields)
	log.Info("test message")

	output := buf.String()
	for k, v := range fields {
		expected := fmt.Sprintf("%s=%v", k, v)
		if !strings.Contains(output, expected) {
			t.Errorf("Expected field %q in output, got: %s", expected, output)
		}
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewLogger(&logger.Options{
		Level:  logger.ERROR,
		Output: &buf,
	})

	log.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("Debug message should not be logged when level is ERROR")
	}

	log.Info("info message")
	if buf.Len() > 0 {
		t.Error("Info message should not be logged when level is ERROR")
	}

	log.Error("error message")
	if buf.Len() == 0 {
		t.Error("Error message should be logged when level is ERROR")
	}
}

func TestDefaultLogger(t *testing.T) {
	logger1 := logger.DefaultLogger()
	logger2 := logger.DefaultLogger()

	if logger1 != logger2 {
		t.Error("DefaultLogger should return the same instance")
	}
}

func TestLoggerClone(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewLogger(&logger.Options{
		Level:  logger.INFO,
		Output: &buf,
	})

	log1 := log.WithField("key1", "value1")
	log2 := log.WithField("key2", "value2")

	log1.Info("test message 1")
	output1 := buf.String()
	if !strings.Contains(output1, "key1=value1") {
		t.Error("Cloned logger should have its own fields")
	}
	if strings.Contains(output1, "key2=value2") {
		t.Error("Cloned logger should not contain fields from other loggers")
	}

	buf.Reset()
	log2.Info("test message 2")
	output2 := buf.String()
	if strings.Contains(output2, "key1=value1") {
		t.Error("Cloned logger should not contain fields from other loggers")
	}
	if !strings.Contains(output2, "key2=value2") {
		t.Error("Cloned logger should have its own fields")
	}
}
