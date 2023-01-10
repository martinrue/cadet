package cadet

import (
	"encoding/json"
	"net/http"
)

type Response func(w http.ResponseWriter)

func JSON(response any) Response {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(response)
	}
}

func Text(text string) Response {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(text))
	}
}

func Status(status int) Response {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
	}
}

func Error(status int, message string) Response {
	return func(w http.ResponseWriter) {
		type response struct {
			Error string `json:"error"`
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		JSON(&response{message})(w)
	}
}
