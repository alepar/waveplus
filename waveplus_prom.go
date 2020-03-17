package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux"
	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alepar/airthings/airthings/waveplus"
)

// CLI args
var (
	listenAddr   = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	readInterval = flag.Duration("read-int", 30*time.Second, "time interval between sensor reads")
	scanDuration = flag.Duration("scan-dur", 5000*time.Millisecond, "scan duration")
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

	//logging
	formatter := &log.TextFormatter{
		FullTimestamp: true,
	}
	log.SetFormatter(formatter)
}

func main() {
	flag.Parse()

	// TODO logs if it started successfully

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
		scanAndReceive()
		time.Sleep(*readInterval)
	}
}

func scanAndReceive() {
	// open BLE
	d, err := linux.NewDevice()
	if err != nil {
		log.Errorf("failed to open ble: %s", err)
		return
	}
	ble.SetDefaultDevice(d)
	defer ble.Stop()

	// Scan
	scanner := waveplus.BleScanner{
		ScanDuration: *scanDuration,
		Retries:      *retries,
	}
	sensorsMap, err := scanner.Scan()
	if err != nil {
		log.Errorf("failed to scan for sensors: %s", err)
		return
	}

	// Receive from every found sensor
	for serialNr, sensor := range sensorsMap {
		log.Printf("Found: serialNr %s addr %s", serialNr, sensor.Address())

		values, err := sensor.Receive()
		if err != nil {
			log.Errorf("failed to read from sensor (serialNr %s): %s", serialNr, err)
			continue
		}

		valuesAsJson, err := json.Marshal(values)
		if err == nil {
			log.Printf("Received: %s", valuesAsJson)
		} else {
			log.Printf("Received: <marshall error: %s>", err)
		}

		gaugeHumidity.WithLabelValues(serialNr).Set(float64(values.Humidity))
		gaugeRadonShort.WithLabelValues(serialNr).Set(float64(values.RadonShort))
		gaugeRadonLong.WithLabelValues(serialNr).Set(float64(values.RadonLong))
		gaugeTemperature.WithLabelValues(serialNr).Set(float64(values.Temperature))
		gaugeAtmPressure.WithLabelValues(serialNr).Set(float64(values.AtmPressure))
		gaugeCo2Level.WithLabelValues(serialNr).Set(float64(values.Co2Level))
		gaugeVocLevel.WithLabelValues(serialNr).Set(float64(values.VocLevel))

		// TODO metric and log for a successful/failed read from sensor
		// TODO when read failed - stop gauge from being reported to prometheus - ie you get missing data points
		// TODO how about panicking when all retries exhausted? doublecheck it kills the process? or recovers
	}
}
