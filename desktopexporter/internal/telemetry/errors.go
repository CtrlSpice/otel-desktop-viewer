package telemetry

import (
	"errors"
)

var ErrMissingRootSpan = errors.New("warning: trace is incomplete - no root span found")
var ErrInvalidServiceName = errors.New("warning: Resource.Attributes['service.name'] must be a string value that helps to distinguish a group of services")
