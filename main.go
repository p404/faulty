package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/github/go-fault"
	"github.com/gorilla/mux"
)

var (
	externalService = flag.String("microservice", "http://www.google.com", "Microservice Endpoint")
	faults          = flag.Bool("faults", false, "Active/disable faults")
	percent         = flag.Float64("percent", 0, "Percentage of faults")
	faultType       = flag.String("type", "error", "Type of the value")
	latency         = flag.Int64("latency", 1, "Latency in milliseconds to be added to fault type slowness")
)

func init() {
	flag.Parse()
}

func handleAuthz(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Security!")
}

func handleMicroservice(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(*externalService)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Println("Response status:", resp.Status)
	fmt.Fprintf(w, "External service!")
}

func Router() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/authz", handleAuthz).Methods("GET")
	r.HandleFunc("/microservice", handleMicroservice).Methods("GET")
	return r
}

func main() {
	var latencyDuration time.Duration
	latencyDuration = time.Duration(*latency)
	var errorInjector fault.Injector

	if strings.TrimRight(*faultType, "\n") == "error" {
		errorInjector, _ = fault.NewErrorInjector(500)
	} else if strings.TrimRight(*faultType, "\n") == "slowness" {
		errorInjector, _ = fault.NewSlowInjector(time.Millisecond * latencyDuration)
	}

	errorFault, _ := fault.NewFault(errorInjector,
		fault.WithEnabled(*faults),
		fault.WithParticipation(float32(*percent)),
		fault.WithPathBlocklist([]string{"/ping", "/health"}),
	)

	handlerChain := errorFault.Handler(Router())
	log.Fatal(http.ListenAndServe(":8080", handlerChain))
}
