package anomaly

import "errors"

// ErrInvalidWindow is returned when a scoring window is missing or not
// end-after-start.
var ErrInvalidWindow = errors.New("anomaly: invalid window")

// ErrTenantMismatch is returned when a telemetry row belongs to a merchant
// other than the one being scored. Cross-tenant mixing is never allowed.
var ErrTenantMismatch = errors.New("anomaly: tenant mismatch")

// ErrDisabled is returned when scoring is attempted while the model is
// disabled (score must stay null in that case).
var ErrDisabled = errors.New("anomaly: scoring disabled")
