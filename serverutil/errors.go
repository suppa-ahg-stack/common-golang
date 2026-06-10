package serverutil

import "errors"

var CsrfErrors = struct {
	NotFoundInSessionCache error
	DoesntExistInCache     error
	Expired                error
	Invalid                error
}{
	NotFoundInSessionCache: errors.New("no csrf found for the entry in the session cache"),
	DoesntExistInCache:     errors.New("session must exist in cache to check Csrf token"),
	Expired:                errors.New("csrf expired"),
	Invalid:                errors.New("csrf invalid"),
}
