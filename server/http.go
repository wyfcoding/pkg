package server

import (
	"net/http"
	"time"
)

type HttpServer struct {
	server *http.Server
}

func NewHttpServer(addr string, handler http.Handler) *HttpServer {
	return &HttpServer{
		server: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

func (s *HttpServer) Run() error {
	return s.server.ListenAndServe()
}
