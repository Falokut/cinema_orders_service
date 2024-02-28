package models

import (
	"errors"
	"fmt"
)

type ErrorCode int32

const (
	Unknown ErrorCode = iota
	Internal
	InvalidArgument
	Unauthenticated
	Conflict
	NotFound
	Canceled
	DeadlineExceeded
	PermissionDenied
)

type ServiceError struct {
	Msg  string
	Code ErrorCode
}

func (t ErrorCode) String() string {
	switch t {
	case Internal:
		return "Internal"
	case InvalidArgument:
		return "InvalidArgument"
	case Unauthenticated:
		return "Unauthenticated"
	case Conflict:
		return "Conflict"
	case NotFound:
		return "NotFound"
	case Canceled:
		return "Canceled"
	case DeadlineExceeded:
		return "DeadlineExceeded"
	default:
		return "Unknown"
	}
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("%s %s", e.Code, e.Msg)
}

func Code(err error) ErrorCode {
	srvErr := &ServiceError{}
	if errors.As(err, &srvErr) {
		return srvErr.Code
	}
	return Unknown
}
func Error(code ErrorCode, msg string) *ServiceError {
	return &ServiceError{Code: code, Msg: msg}
}
func Errorf(code ErrorCode, format string, a ...any) *ServiceError {
	return &ServiceError{Code: code, Msg: fmt.Sprintf(format, a...)}
}
