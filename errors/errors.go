package errors

import (
	"fmt"
)

// ErrorCode 定义错误码类型
type ErrorCode int

const (
	// 错误码定义
	ErrCodeUnknown ErrorCode = iota
	ErrCodeConfigInvalid
	ErrCodeServiceNotFound
	ErrCodeNoHealthyInstances
	ErrCodeConsulNotAvailable
	ErrCodeEmptyServiceName
	ErrCodeNothingToRefresh
	ErrCodeTimeout
	ErrCodeValidation
)

// ConsulError 统一的错误类型
type ConsulError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *ConsulError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// 预定义错误
var (
	ErrServiceNotFound    = &ConsulError{Code: ErrCodeServiceNotFound, Message: "service not found"}
	ErrNoHealthyInstances = &ConsulError{Code: ErrCodeNoHealthyInstances, Message: "no healthy instances available"}
	ErrConsulNotAvailable = &ConsulError{Code: ErrCodeConsulNotAvailable, Message: "consul service not available"}
	ErrEmptyServiceName   = &ConsulError{Code: ErrCodeEmptyServiceName, Message: "empty service name"}
	ErrNothingToRefresh   = &ConsulError{Code: ErrCodeNothingToRefresh, Message: "nothing to refresh"}
	ErrTimeout            = &ConsulError{Code: ErrCodeTimeout, Message: "operation timeout"}
)

// NewError 创建新的错误
func NewError(code ErrorCode, message string, err error) *ConsulError {
	return &ConsulError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// IsConsulError 判断是否为 ConsulError 类型
func IsConsulError(err error) bool {
	_, ok := err.(*ConsulError)
	return ok
}

// GetErrorCode 获取错误码
func GetErrorCode(err error) ErrorCode {
	if consulErr, ok := err.(*ConsulError); ok {
		return consulErr.Code
	}
	return ErrCodeUnknown
}
