package protocol

import "fmt"

// ProtocolError represents a wire-level protocol error.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string { return e.Message }

// ErrProtocol creates a new protocol error.
func ErrProtocol(format string, args ...any) *ProtocolError {
	return &ProtocolError{Message: fmt.Sprintf(format, args...)}
}

// Common RESP error strings.
const (
	RespOK              = "+OK\r\n"
	RespPong            = "+PONG\r\n"
	RespNilBulk         = "$-1\r\n"
	RespNilArray        = "*-1\r\n"
	RespZero            = ":0\r\n"
	RespOne             = ":1\r\n"
	RespNegOne          = ":-1\r\n"
	ErrWrongNumArgs     = "ERR wrong number of arguments for '%s' command"
	ErrNotInteger       = "ERR value is not an integer or out of range"
	ErrSyntax           = "ERR syntax error"
	ErrWrongType        = "WRONGTYPE Operation against a key holding the wrong kind of value"
	ErrNoAuth           = "NOAUTH Authentication required"
	ErrNoPermission     = "NOPERM this user has no permissions to run the '%s' command"
	ErrOutOfMemory      = "OOM command not allowed when used memory > 'maxmemory'"
	ErrBusy             = "BUSY"
	ErrExecAbort        = "EXECABORT Transaction discarded because of previous errors"
)

// WrongNumArgsError returns a formatted wrong-num-args error string.
func WrongNumArgsError(cmd string) string {
	return fmt.Sprintf(ErrWrongNumArgs, cmd)
}
