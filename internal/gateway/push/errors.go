package push

import (
	"errors"
	"time"
)

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrUnavailable    = errors.New("push service is unavailable")
)

type TooManyRequests struct {
	RetryAfter time.Time
}

func (TooManyRequests) Error() string { return "push service rate limit exceeded" }
