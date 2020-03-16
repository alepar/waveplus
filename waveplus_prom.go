package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alepar/airthings/airthings/waveplus"
)

// CLI args
var (
	listenAddr   = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	readInterval = flag.Duration("read-int", 1*time.Minute, "time interval between sensor reads")
	scanDuration = flag.Duration("scan-dur", 2500*time.Millisecond, "scan duration")
	retries      = flag.Int("retries", 5, "max number of tries in case of BLE errors")
)

// metrics to expose to Prometheus
var (
	gaugeHumidity    = newGauge("air_humidity", "Humidity (units: % of relative Humidity)")
	gaugeRadonShort  = newGauge("air_radon_short", "Radon Short Term estimate (units: Bq/m3)")
	gaugeRadonLong   = newGauge("air_radon_long", "Radon Long Term estimate (units: Bq/m3)")
	gaugeTemperature = newGauge("air_temperature", "Air Temperature (units: degrees Celsius)")
	gaugeAtmPressure = newGauge("air_atm_pressure", "Atmospheric Pressure (units: hPa)")
	gaugeCo2Level    = newGauge("air_co2_level", "Air Carbon Dioxide level (units: ppm)")
	gaugeVocLevel    = newGauge("air_voc_level", "Air Volatile Organic Compounds level (units: ppb)")
)

func newGauge(name string, help string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: help,
		},
		[]string{"serial_number"},
	)
}

func init() {
	prometheus.MustRegister(gaugeHumidity)
	prometheus.MustRegister(gaugeRadonShort)
	prometheus.MustRegister(gaugeRadonLong)
	prometheus.MustRegister(gaugeTemperature)
	prometheus.MustRegister(gaugeAtmPressure)
	prometheus.MustRegister(gaugeCo2Level)
	prometheus.MustRegister(gaugeVocLevel)

	// Add Go module build info.
	prometheus.MustRegister(prometheus.NewBuildInfoCollector())
}

func main() {
	flag.Parse()

	scanner := waveplus.BleScanner{
		ScanDuration: *scanDuration,
		Retries:      *retries,
	}
	sensorsMap, err := scanner.Scan()
	if err != nil {
		panic(err)
	}

	go func() {
		// Expose the registered metrics via HTTP.
		http.Handle("/metrics", promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{
				// Opt into OpenMetrics to support exemplars.
				EnableOpenMetrics: true,
			},
		))
		log.Panic(http.ListenAndServe(*listenAddr, nil))
	}()

	for {
		for serialNr, sensor := range sensorsMap {
			values, err := sensor.Receive()
			if err != nil {
				panic(err)
			}

			gaugeHumidity.WithLabelValues(serialNr).Set(float64(values.Humidity))
			gaugeRadonShort.WithLabelValues(serialNr).Set(float64(values.RadonShort))
			gaugeRadonLong.WithLabelValues(serialNr).Set(float64(values.RadonLong))
			gaugeTemperature.WithLabelValues(serialNr).Set(float64(values.Temperature))
			gaugeAtmPressure.WithLabelValues(serialNr).Set(float64(values.AtmPressure))
			gaugeCo2Level.WithLabelValues(serialNr).Set(float64(values.Co2Level))
			gaugeVocLevel.WithLabelValues(serialNr).Set(float64(values.VocLevel))
		}

		time.Sleep(*readInterval)
	}
}
