package agent

import "errors"

var errNoRecorder = errors.New("finding recorder is not configured")

// errDeskUnconfirmed means a strict fireteam has not been confirmed.
var errDeskUnconfirmed = errors.New("desk: strict fireteam is not confirmed")
