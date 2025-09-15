package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/serviceerp/inwx-client-go"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const version = "dirty"

var system = flag.String("system", "OTE", "inwx system (LIVE,OTE)")
var username = flag.String("username", "", "inwx username")
var password = flag.String("password", "", "inwx password")

type metrics struct {
	domainExpiration *prometheus.GaugeVec
	domainCount      prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		domainExpiration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "inwx_domain_expiration_hours",
			Help: "Domain Expiration in hours left",
		}, []string{"domain"}),
		domainCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "inwx_domain_count",
			Help: "Count of registered domains at inwx",
		}),
	}
	reg.MustRegister(m.domainExpiration)
	reg.MustRegister(m.domainCount)
	return m
}

func refresh(m *metrics) {
	log.Println("Refreshing Data")

	var apiUrl = inwx.API_OTE_URL
	if *system == "LIVE" {
		apiUrl = inwx.API_LIVE_URL
	}

	inwxClient, err := inwx.NewInwxClient(apiUrl, inwx.InwxClientOptions{})
	if err != nil {
		panic(err)
	}

	err = inwxClient.AccountLogin(*username, *password, "", "")
	if err != nil {
		panic(err)
	}

	response, err := inwxClient.DomainList()
	if err != nil {
		panic(err)
	}

	m.domainCount.Set(float64((*response).Response.Count))
	for _, domain := range (*response).Response.Domain {
		m.domainExpiration.With(prometheus.Labels{"domain": domain.Domain}).Set(domain.HoursLeft())
	}
}

func main() {
	log.Printf("inwx-exporter - %s", version)

	flag.Parse()

	portEnv, exists := os.LookupEnv("PORT")

	if !exists {
		portEnv = "9412"
	}

	port, err := strconv.Atoi(portEnv)
	if err != nil {
		panic(err)
	}

	reg := prometheus.NewRegistry()
	m := newMetrics(reg)

	refresh(m)

	ticker := time.NewTicker(60 * time.Minute)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				refresh(m)
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	err = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		panic(err)
	}
}
