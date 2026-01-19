package main

import (
	"net/http"
	cli "orbo/gen/http/cli/orbo"
	"time"

	goahttp "goa.design/goa/v3/http"
	goa "goa.design/goa/v3/pkg"
)

func doHTTP(scheme, host string, timeout int, debug bool) (goa.Endpoint, any, error) {
	var (
		doer goahttp.Doer
	)
	{
		doer = &http.Client{Timeout: time.Duration(timeout) * time.Second}
		if debug {
			doer = goahttp.NewDebugDoer(doer)
		}
	}

	return cli.ParseEndpoint(
		scheme,
		host,
		doer,
		goahttp.RequestEncoder,
		goahttp.ResponseDecoder,
		debug,
	)
}

func httpUsageCommands() string {
	commands := cli.UsageCommands()
	result := ""
	for i, cmd := range commands {
		if i > 0 {
			result += "\n"
		}
		result += cmd
	}
	return result
}

func httpUsageExamples() string {
	return cli.UsageExamples()
}
