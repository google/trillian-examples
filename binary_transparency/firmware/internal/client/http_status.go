package client

import (
	"net/http"

	"google.golang.org/grpc/codes"
)

func codeFromHTTPResponse(r int) codes.Code {
	switch r {
	case http.StatusOK:
		return codes.OK
	case http.StatusRequestTimeout:
		return codes.Canceled
	case http.StatusInternalServerError:
		return codes.Internal
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusGatewayTimeout:
		return codes.DeadlineExceeded
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusNotImplemented:
		return codes.Unimplemented
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	default:
		return codes.Unknown
	}
}
