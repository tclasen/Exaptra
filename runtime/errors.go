package runtime

import (
	"errors"
	"fmt"
)

// ErrorCategory classifies runtime failures so callers can branch on the
// failure domain without parsing error strings.
type ErrorCategory string

const (
	ErrorCategoryModel      ErrorCategory = "model"
	ErrorCategoryTool       ErrorCategory = "tool"
	ErrorCategoryConfig     ErrorCategory = "config"
	ErrorCategoryPermission ErrorCategory = "permission"
	ErrorCategoryTransition ErrorCategory = "transition"
)

// Error is the structured error returned by the runtime layer.
type Error struct {
	Category ErrorCategory `json:"category"`
	Op       string        `json:"op,omitempty"`
	Message  string        `json:"message"`
	Err      error         `json:"-"`
}

func (e *Error) Error() string {
	switch {
	case e == nil:
		return "<nil>"
	case e.Op != "" && e.Err != nil:
		return fmt.Sprintf("%s %s: %s: %v", e.Category, e.Op, e.Message, e.Err)
	case e.Op != "":
		return fmt.Sprintf("%s %s: %s", e.Category, e.Op, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%s: %s: %v", e.Category, e.Message, e.Err)
	default:
		return fmt.Sprintf("%s: %s", e.Category, e.Message)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewError constructs a structured runtime error for the given category.
func NewError(category ErrorCategory, op, message string, err error) error {
	return &Error{
		Category: category,
		Op:       op,
		Message:  message,
		Err:      err,
	}
}

// AsError extracts a structured runtime error from err when possible.
func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

func NewModelError(op, message string, err error) error {
	return NewError(ErrorCategoryModel, op, message, err)
}

func NewToolError(op, message string, err error) error {
	return NewError(ErrorCategoryTool, op, message, err)
}

func NewConfigError(op, message string, err error) error {
	return NewError(ErrorCategoryConfig, op, message, err)
}

func NewPermissionError(op, message string, err error) error {
	return NewError(ErrorCategoryPermission, op, message, err)
}

func NewTransitionError(op, message string, err error) error {
	return NewError(ErrorCategoryTransition, op, message, err)
}
