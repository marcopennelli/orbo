package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	camera "orbo/gen/camera"
	config "orbo/gen/config"
	health "orbo/gen/health"
	camerasvr "orbo/gen/http/camera/server"
	configsvr "orbo/gen/http/config/server"
	healthsvr "orbo/gen/http/health/server"
	motionsvr "orbo/gen/http/motion/server"
	systemsvr "orbo/gen/http/system/server"
	motion "orbo/gen/motion"
	system "orbo/gen/system"
	"os"
	"sync"
	"time"

	goahttp "goa.design/goa/v3/http"
	httpmdlwr "goa.design/goa/v3/http/middleware"
	"goa.design/goa/v3/middleware"
)

// handleHTTPServer starts configures and starts a HTTP server on the given
// URL. It shuts down the server if any error is received in the error channel.
func handleHTTPServer(ctx context.Context, u *url.URL, healthEndpoints *health.Endpoints, cameraEndpoints *camera.Endpoints, motionEndpoints *motion.Endpoints, configEndpoints *config.Endpoints, systemEndpoints *system.Endpoints, wg *sync.WaitGroup, errc chan error, logger *log.Logger, debug bool) {

	// Setup goa log adapter.
	var (
		adapter middleware.Logger
	)
	{
		adapter = middleware.NewLogger(logger)
	}

	// Provide the transport specific request decoder and response encoder.
	// The goa http package has built-in support for JSON, XML and gob.
	// Other encodings can be used by providing the corresponding functions,
	// see goa.design/implement/encoding.
	var (
		dec = goahttp.RequestDecoder
		enc = goahttp.ResponseEncoder
	)

	// Build the service HTTP request multiplexer and configure it to serve
	// HTTP requests to the service endpoints.
	var mux goahttp.Muxer
	{
		mux = goahttp.NewMuxer()
	}

	// Wrap the endpoints with the transport specific layers. The generated
	// server packages contains code generated from the design which maps
	// the service input and output data structures to HTTP requests and
	// responses.
	var (
		healthServer *healthsvr.Server
		cameraServer *camerasvr.Server
		motionServer *motionsvr.Server
		configServer *configsvr.Server
		systemServer *systemsvr.Server
	)
	{
		eh := errorHandler(logger)
		healthServer = healthsvr.New(healthEndpoints, mux, dec, enc, eh, nil)
		cameraServer = camerasvr.New(cameraEndpoints, mux, dec, enc, eh, nil)
		
		// Only create servers for implemented services
		if motionEndpoints != nil {
			motionServer = motionsvr.New(motionEndpoints, mux, dec, enc, eh, nil)
		}
		if configEndpoints != nil {
			configServer = configsvr.New(configEndpoints, mux, dec, enc, eh, nil)
		}
		if systemEndpoints != nil {
			systemServer = systemsvr.New(systemEndpoints, mux, dec, enc, eh, nil)
		}
		
		if debug {
			servers := goahttp.Servers{healthServer, cameraServer}
			if motionServer != nil {
				servers = append(servers, motionServer)
			}
			if configServer != nil {
				servers = append(servers, configServer)
			}
			if systemServer != nil {
				servers = append(servers, systemServer)
			}
			servers.Use(httpmdlwr.Debug(mux, os.Stdout))
		}
	}
	// Configure the mux.
	healthsvr.Mount(mux, healthServer)
	camerasvr.Mount(mux, cameraServer)
	if motionServer != nil {
		motionsvr.Mount(mux, motionServer)
	}
	if configServer != nil {
		configsvr.Mount(mux, configServer)
	}
	if systemServer != nil {
		systemsvr.Mount(mux, systemServer)
	}

	// Wrap the multiplexer with additional middlewares. Middlewares mounted
	// here apply to all the service endpoints.
	var handler http.Handler = mux
	{
		handler = httpmdlwr.Log(adapter)(handler)
		handler = httpmdlwr.RequestID()(handler)
	}

	// Start HTTP server using default configuration, change the code to
	// configure the server as required by your service.
	srv := &http.Server{Addr: u.Host, Handler: handler, ReadHeaderTimeout: time.Second * 60}
	for _, m := range healthServer.Mounts {
		logger.Printf("HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range cameraServer.Mounts {
		logger.Printf("HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	if motionServer != nil {
		for _, m := range motionServer.Mounts {
			logger.Printf("HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
		}
	}
	if configServer != nil {
		for _, m := range configServer.Mounts {
			logger.Printf("HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
		}
	}
	if systemServer != nil {
		for _, m := range systemServer.Mounts {
			logger.Printf("HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
		}
	}

	(*wg).Add(1)
	go func() {
		defer (*wg).Done()

		// Start HTTP server in a separate goroutine.
		go func() {
			logger.Printf("HTTP server listening on %q", u.Host)
			errc <- srv.ListenAndServe()
		}()

		<-ctx.Done()
		logger.Printf("shutting down HTTP server at %q", u.Host)

		// Shutdown gracefully with a 30s timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := srv.Shutdown(ctx)
		if err != nil {
			logger.Printf("failed to shutdown: %v", err)
		}
	}()
}

// errorHandler returns a function that writes and logs the given error.
// The function also writes and logs the error unique ID so that it's possible
// to correlate.
func errorHandler(logger *log.Logger) func(context.Context, http.ResponseWriter, error) {
	return func(ctx context.Context, w http.ResponseWriter, err error) {
		id := ctx.Value(middleware.RequestIDKey).(string)
		_, _ = w.Write([]byte("[" + id + "] encoding: " + err.Error()))
		logger.Printf("[%s] ERROR: %s", id, err.Error())
	}
}
