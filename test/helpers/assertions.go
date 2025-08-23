package helpers

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Assertions provides custom assertion functions for testing
type Assertions struct {
	t *testing.T
}

// NewAssertions creates a new assertions helper
func NewAssertions(t *testing.T) *Assertions {
	return &Assertions{t: t}
}

// Equal asserts that two values are equal
func (a *Assertions) Equal(expected, actual interface{}, msgAndArgs ...interface{}) {
	if !reflect.DeepEqual(expected, actual) {
		msg := fmt.Sprintf("Expected %v, got %v", expected, actual)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// NotEqual asserts that two values are not equal
func (a *Assertions) NotEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	if reflect.DeepEqual(expected, actual) {
		msg := fmt.Sprintf("Expected values to be different, but both were %v", expected)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Nil asserts that the value is nil
func (a *Assertions) Nil(value interface{}, msgAndArgs ...interface{}) {
	if value != nil {
		msg := fmt.Sprintf("Expected nil, got %v", value)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// NotNil asserts that the value is not nil
func (a *Assertions) NotNil(value interface{}, msgAndArgs ...interface{}) {
	if value == nil {
		msg := "Expected value to not be nil"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// True asserts that the value is true
func (a *Assertions) True(value bool, msgAndArgs ...interface{}) {
	if !value {
		msg := "Expected true, got false"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// False asserts that the value is false
func (a *Assertions) False(value bool, msgAndArgs ...interface{}) {
	if value {
		msg := "Expected false, got true"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Contains asserts that the string contains the substring
func (a *Assertions) Contains(str, substr string, msgAndArgs ...interface{}) {
	if !strings.Contains(str, substr) {
		msg := fmt.Sprintf("Expected %q to contain %q", str, substr)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// NotContains asserts that the string does not contain the substring
func (a *Assertions) NotContains(str, substr string, msgAndArgs ...interface{}) {
	if strings.Contains(str, substr) {
		msg := fmt.Sprintf("Expected %q to not contain %q", str, substr)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Error asserts that the error is not nil
func (a *Assertions) Error(err error, msgAndArgs ...interface{}) {
	if err == nil {
		msg := "Expected an error, got nil"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// NoError asserts that the error is nil
func (a *Assertions) NoError(err error, msgAndArgs ...interface{}) {
	if err != nil {
		msg := fmt.Sprintf("Expected no error, got %v", err)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// ErrorContains asserts that the error contains the expected message
func (a *Assertions) ErrorContains(err error, expectedMsg string, msgAndArgs ...interface{}) {
	if err == nil {
		msg := fmt.Sprintf("Expected error containing %q, got nil", expectedMsg)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
		return
	}

	if !strings.Contains(err.Error(), expectedMsg) {
		msg := fmt.Sprintf("Expected error to contain %q, got %q", expectedMsg, err.Error())
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Greater asserts that the first value is greater than the second
func (a *Assertions) Greater(x, y interface{}, msgAndArgs ...interface{}) {
	if !isGreater(x, y) {
		msg := fmt.Sprintf("Expected %v to be greater than %v", x, y)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// GreaterOrEqual asserts that the first value is greater than or equal to the second
func (a *Assertions) GreaterOrEqual(x, y interface{}, msgAndArgs ...interface{}) {
	if !isGreaterOrEqual(x, y) {
		msg := fmt.Sprintf("Expected %v to be greater than or equal to %v", x, y)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Less asserts that the first value is less than the second
func (a *Assertions) Less(x, y interface{}, msgAndArgs ...interface{}) {
	if !isLess(x, y) {
		msg := fmt.Sprintf("Expected %v to be less than %v", x, y)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// WithinDuration asserts that the time difference is within the expected duration
func (a *Assertions) WithinDuration(expected, actual time.Time, delta time.Duration, msgAndArgs ...interface{}) {
	diff := actual.Sub(expected)
	if diff < 0 {
		diff = -diff
	}

	if diff > delta {
		msg := fmt.Sprintf("Expected time difference to be within %v, got %v", delta, diff)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Eventually asserts that the condition becomes true within the timeout
func (a *Assertions) Eventually(condition func() bool, timeout time.Duration, msgAndArgs ...interface{}) {
	start := time.Now()
	for time.Since(start) < timeout {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	msg := fmt.Sprintf("Condition was not met within %v", timeout)
	if len(msgAndArgs) > 0 {
		msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
	}
	a.t.Error(msg)
}

// HTTPStatusCode asserts that the HTTP response has the expected status code
func (a *Assertions) HTTPStatusCode(response *Response, expectedStatus int, msgAndArgs ...interface{}) {
	if response.StatusCode != expectedStatus {
		msg := fmt.Sprintf("Expected HTTP status %d, got %d", expectedStatus, response.StatusCode)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// HTTPBodyContains asserts that the HTTP response body contains the expected string
func (a *Assertions) HTTPBodyContains(response *Response, expectedSubstring string, msgAndArgs ...interface{}) {
	if !strings.Contains(response.Body, expectedSubstring) {
		msg := fmt.Sprintf("Expected response body to contain %q, got %q", expectedSubstring, response.Body)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Metrics asserts that a metric has the expected value
func (a *Assertions) MetricValue(metricValue float64, expectedValue float64, msgAndArgs ...interface{}) {
	if metricValue != expectedValue {
		msg := fmt.Sprintf("Expected metric value %f, got %f", expectedValue, metricValue)
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(fmt.Sprint(msgAndArgs[0]), msgAndArgs[1:]...) + ": " + msg
		}
		a.t.Error(msg)
	}
}

// Helper functions for comparison

func isGreater(x, y interface{}) bool {
	return compare(x, y) > 0
}

func isGreaterOrEqual(x, y interface{}) bool {
	return compare(x, y) >= 0
}

func isLess(x, y interface{}) bool {
	return compare(x, y) < 0
}

func compare(x, y interface{}) int {
	switch xv := x.(type) {
	case int:
		if yv, ok := y.(int); ok {
			if xv > yv {
				return 1
			} else if xv < yv {
				return -1
			}
			return 0
		}
	case int64:
		if yv, ok := y.(int64); ok {
			if xv > yv {
				return 1
			} else if xv < yv {
				return -1
			}
			return 0
		}
	case float64:
		if yv, ok := y.(float64); ok {
			if xv > yv {
				return 1
			} else if xv < yv {
				return -1
			}
			return 0
		}
	case time.Duration:
		if yv, ok := y.(time.Duration); ok {
			if xv > yv {
				return 1
			} else if xv < yv {
				return -1
			}
			return 0
		}
	}
	return 0
}
