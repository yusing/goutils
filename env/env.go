package env

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var envPrefixes = []string{"GODOXY_", "GOPROXY_", ""}

func SetPrefixes(prefixes ...string) {
	if len(prefixes) == 0 {
		envPrefixes = []string{""}
		return
	}
	envPrefixes = prefixes
}

func GetEnv[T any](key string, defaultValue T, parser func(string) (T, error)) T {
	var value string
	var ok bool
	for _, prefix := range envPrefixes {
		value, ok = os.LookupEnv(prefix + key)
		if ok && value != "" {
			break
		}
	}
	if !ok || value == "" {
		return defaultValue
	}
	parsed, err := parser(value)
	if err == nil {
		return parsed
	}
	log.Panicf("env %s: invalid %T value: %s", key, parsed, value)
	return defaultValue
}

func stringstring(s string) (string, error) {
	return s, nil
}

func GetEnvString(key string, defaultValue string) string {
	return GetEnv(key, defaultValue, stringstring)
}

func GetEnvBool(key string, defaultValue bool) bool {
	return GetEnv(key, defaultValue, strconv.ParseBool)
}

func GetEnvInt(key string, defaultValue int) int {
	return GetEnv(key, defaultValue, strconv.Atoi)
}

func GetAddrEnv(key, defaultValue, scheme string) (addr, host string, portInt int, fullURL string) {
	addr = GetEnvString(key, defaultValue)
	if addr == "" {
		return
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Panicf("env %s: invalid address: %s", key, addr)
	}
	if host == "" {
		host = "localhost"
	}
	fullURL = fmt.Sprintf("%s://%s:%s", scheme, host, port)
	portInt, err = strconv.Atoi(port)
	if err != nil {
		log.Panicf("env %s: invalid port: %s", key, port)
	}
	return
}

func GetEnvDuation(key string, defaultValue time.Duration) time.Duration {
	return GetEnv(key, defaultValue, time.ParseDuration)
}

func GetEnvCommaSep(key string, defaultValue string) []string {
	strs := strings.Split(GetEnvString(key, defaultValue), ",")
	for i, str := range strs {
		strs[i] = strings.TrimSpace(str)
	}
	return strs
}
