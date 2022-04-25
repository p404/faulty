package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/github/go-fault"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

const serviceName = "faulty"

var (
	externalService = flag.String("microservice", "http://www.google.com", "Microservice Endpoint")
	faults          = flag.Bool("faults", false, "Active/disable faults")
	percent         = flag.Float64("percent", 0, "Percentage of faults")
	faultType       = flag.String("type", "error", "Type of the value")
	latency         = flag.Int64("latency", 1, "Latency in milliseconds to be added to fault type slowness")
	collectorAddr   = flag.String("collector", "http://localhost:14268/api/traces", "Collector endpoint")
)

func init() {
	flag.Parse()
}

func setupTracing() func(context.Context) {
	tp, err := tracerProvider(*collectorAddr)
	if err != nil {
		log.Fatal(err)
	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
		b3.New(),
	))

	return func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}
}

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)
	return tp, nil
}

func handleAuthz(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Security!")
	log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
}

func handleMicroservice(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(*externalService)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Fprintf(w, "External service!")
	log.Printf("External service address: %s with response: %s", *externalService, resp.Status)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown := setupTracing()
	defer shutdown(ctx)

	errorFault, _ := fault.NewFault(errorInjector,
		fault.WithEnabled(*faults),
		fault.WithParticipation(float32(*percent)),
		fault.WithPathBlocklist([]string{"/ping", "/health"}),
	)

	handlerChain := errorFault.Handler(Router())
	log.Println("Faulty is starting...")
	log.Fatal(http.ListenAndServe(":8080", handlerChain))
}
