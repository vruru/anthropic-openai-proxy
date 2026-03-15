package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	config := LoadConfig()
	handler := NewProxyHandler(config)

	wrappedHandler := Chain(
		LoggingMiddleware,
		CORSMiddleware,
	)(handler)

	server := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%s", config.Port),
		Handler:      wrappedHandler,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[anthropic-proxy] listening on port %s", config.Port)
		log.Printf("[anthropic-proxy] gateway: %s", config.GatewayURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[anthropic-proxy] server error: %v", err)
		}
	}()

	<-done
	log.Println("[anthropic-proxy] shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("[anthropic-proxy] shutdown error: %v", err)
	}
	log.Println("[anthropic-proxy] stopped")
}
