package main

import "net/http"

func NewServer() *http.Server {
	mux := http.NewServeMux()
	addRoutes(mux)
	server := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	return &server
}
