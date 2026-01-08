# env

Environment variable utilities with prefix support and type-safe parsing.

## Overview

The `env` package provides functions for reading environment variables with support for multiple prefixes and type-safe parsing.

## API Reference

### Prefix Management

```go
func SetPrefixes(prefixes ...string)
```

### Typed Accessors

```go
func GetEnvString(key string, defaultValue string) string
func GetEnvBool(key string, defaultValue bool) bool
func GetEnvInt(key string, defaultValue int) int
func GetEnvDuation(key string, defaultValue time.Duration) time.Duration
func GetEnvCommaSep(key string, defaultValue string) []string
func GetAddrEnv(key, defaultValue, scheme string) (addr, host string, portInt int, fullURL string)
```

## Usage

```go
// String with default
appName := env.GetEnvString("APP_NAME", "my-app")

// Boolean
debugMode := env.GetEnvBool("DEBUG", false)

// Integer
port := env.GetEnvInt("PORT", 8080)

// Duration
timeout := env.GetEnvDuation("TIMEOUT", 30_000_000_000)

// Comma-separated list
hosts := env.GetEnvCommaSep("HOSTS", "localhost")

// Address parsing
addr, host, port, url := env.GetAddrEnv("SERVER_ADDR", "0.0.0.0:8080", "http")
```

## Lookup Behavior

Looks for `GODOXY_` + key, then `GOPROXY_` + key, then key itself.
