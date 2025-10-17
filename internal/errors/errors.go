package errorz

import "errors"

var ErrErrorWileStartingOTel = errors.New("error while starting OTel")
var ErrErrorWileStoppingOTel = errors.New("error while stopping OTel")
var ErrConfigNotFound = errors.New("config not found")
var ErrServerError = errors.New("server error")
var ErrDatabaseError = errors.New("database error")

var ErrSessionTokenRequired = errors.New("service map session token is required")
var ErrSessionTokenNotFound = errors.New("session token not found")
var ErrInvalidSessionToken = errors.New("invalid session token")
var ErrWhileCreatingSessionToken = errors.New("error while creating session token")
var ErrWhileGettingOtelTraces = errors.New("error while getting otel traces")
var ErrWhileGettingOtelTraceCount = errors.New("error while getting otel trace count")
var ErrWhileGettingOtelTraceErrorCount = errors.New("error while getting otel trace error count")
var ErrWhileBuildingGlobalMetricsAndServices = errors.New("error while building global metrics and services")
