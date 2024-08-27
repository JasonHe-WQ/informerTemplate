package utils

import "net/http"

var Ready = false

// ReadinessHandler Readiness 探针处理函数
func ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	if Ready {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// LivenessHandler Liveness 探针处理函数
func LivenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("alive\n"))
}
