// Package logx writes process logs to the standard logger. Every line starts with trace, debug, info, or error.
// Secrets are removed here so a caller cannot print a bearer token or an sk- key by accident.
package logx

import (
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
)

// Trace records a step inside a call, such as entering the data plane.
func Trace(format string, args ...any) { write("trace", format, args...) }

// Debug records a decision that is useful when a call did not do what the operator expected, such as skipping a deployment.
func Debug(format string, args ...any) { write("debug", format, args...) }

// Info records a normal outcome, such as the process listening or one HTTP request finishing.
func Info(format string, args ...any) { write("info", format, args...) }

// Error records a failure. The process keeps running unless the caller exits after this returns.
func Error(format string, args ...any) { write("error", format, args...) }

func write(level, format string, args ...any) {
	log.Printf("%s %s", level, redact(fmt.Sprintf(format, args...)))
}

var (
	bearerValue = regexp.MustCompile(`(?i)bearer\s+\S+`)
	secretValue = regexp.MustCompile(`sk-[A-Za-z0-9_\-]+`)
	httpURL     = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// redact strips bearer tokens, sk- keys, and full upstream URLs. A URL becomes its hostname.
// The replacement does not itself contain bearer, sk-, or a scheme.
func redact(s string) string {
	s = bearerValue.ReplaceAllString(s, "***")
	s = secretValue.ReplaceAllString(s, "***")
	return httpURL.ReplaceAllStringFunc(s, func(raw string) string {
		u, err := url.Parse(strings.TrimRight(raw, `"'.,);`))
		if err != nil || u.Hostname() == "" {
			return "***"
		}
		return u.Hostname()
	})
}
