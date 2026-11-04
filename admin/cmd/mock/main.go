package main

import (
	"fmt"
	"os"

	"github.com/sjlit/aeus"
	"github.com/sjlit/aeus/transport/http"
)

func main() {
	// Pin default listen port to :8080 so dashboard/web vite proxy
	// (`http://localhost:8080`) works out of the box without env setup.
	// Setting HTTP_PORT before http.New runs lets transport/http's env
	// reader pick it up; if the user already exported HTTP_PORT, honor it.
	if os.Getenv("HTTP_PORT") == "" {
		_ = os.Setenv("HTTP_PORT", "8080")
	}

	scope := NewScopeContext()
	app := aeus.New(
		aeus.WithDebug(true),
		aeus.WithServer(http.New(
			http.WithCORS(),
			http.WithEnableHealth(true), // expose /health and /ready (also JWT-allowlisted)
			// Keep pprof off the public listener; the framework refuses
			// to mount it on the main port unless this loopback address
			// is supplied explicitly.
			http.WithDebugAddr("127.0.0.1:6060"),
		)),
		aeus.WithScope(scope),
		aeus.WithServiceLoader(NewServiceLoader(scope)),
	)
	if err := app.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
