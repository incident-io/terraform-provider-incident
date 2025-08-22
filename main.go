package main

import (
	"context"
	"flag"
	"log"
	"os"

	"net/http"
	"net/http/pprof"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/incident-io/terraform-provider-incident/internal/provider"
)

// Format terraform and generate docs:
//go:generate terraform fmt -recursive ./examples/
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

var (
	version string = "dev" // set by goreleaser
)

func main() {
	// If having performance issues, enable this envar and connect using:
	// go tool pprof localhost:3333
	if os.Getenv("INCIDENT_PROVIDER_PROFILE") == "1" {
		mux := http.NewServeMux()

		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		srv := &http.Server{Addr: "127.0.0.1:3333"}
		srv.Handler = mux

		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Println(err.Error())
			}
		}()
	}

	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/incident-io/incident",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
