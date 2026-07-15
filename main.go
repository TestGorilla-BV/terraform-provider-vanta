package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/provider"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Run the provider in debug mode (attach to a Terraform CLI for tracing).")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/TestGorilla-BV/vanta",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
