package serverutil

import "net/http"

func IsSpaRequest(r *http.Request) bool {
	return r.Header.Get("Accept") == "application/json" ||
		r.Header.Get("X-Requested-With") == "XMLHttpRequest"
}
