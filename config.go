package main

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	GatewayURL   string
	GatewayKey   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func LoadConfig() *Config {
	port := os.Getenv("PROXY_PORT")
	if port == "" {
		port = "18900"
	}

	// Validate port is numeric
	if _, err := strconv.Atoi(port); err != nil {
		port = "18900"
	}

	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "https://ai-gateway.happycapy.ai/api/v1/chat/completions"
	}

	gatewayKey := os.Getenv("GATEWAY_KEY")

	return &Config{
		Port:         port,
		GatewayURL:   gatewayURL,
		GatewayKey:   gatewayKey,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long timeout for streaming
		IdleTimeout:  120 * time.Second,
	}
}
