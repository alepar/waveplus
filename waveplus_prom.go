package main

import (
	"encoding/json"
	"flag"
	"math"
	"net/http"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alepar/airthings/airthings/waveplus"
)

// CLI args
var (
	listenAddr   = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	readInterval = flag.Duration("read-int", 150*time.Second, "time interval between sensor reads")
	scanDuration = flag.Duration("scan-dur", 5*time.Second, "scan duration")
	retries      = flag.Int("retries", 5, "max number of tries in case of BLE errors")
	debug        = flag.Bool("debug", false, "enable debug logging")
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
	flag.Parse()

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
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	// Expose the metrics to Prometheus
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{
				// Opt into OpenMetrics to support exemplars.
				EnableOpenMetrics: true,
			},
		))
		log.Panic(http.ListenAndServe(*listenAddr, nil))
	}()

	watchdogChannel := make(chan bool)
	maxTimeBetweenReads := math.Max(
		(150 * time.Second).Seconds(), // Wave+ updates values every 5min, so we should be reading ~twice as fast
		3*(readInterval.Seconds()+scanDuration.Seconds()),      // or a bit slower than the requested read frequency
	)
	// Start the watchdog thread
	go func() {
		heartbeatsSinceLastRead := 0
		for {
			val := <-watchdogChannel
			if val {
				// successful sensor read
				heartbeatsSinceLastRead = 0
			} else {
				// a heartbeat
				heartbeatsSinceLastRead++
				if float64(heartbeatsSinceLastRead) > maxTimeBetweenReads {
					log.Fatalf("No data received from sensor for over %d seconds, executing suicide", heartbeatsSinceLastRead)
				}
			}
		}
	}()
	// Start the heartbeat thread
	go func (){
		for {
			time.Sleep(1*time.Second)
			watchdogChannel <- false
		}
	}()

	// let's open BLE device and hang on to it
	// not great if we need to share BLE device with other apps
	// but it prevents us from freezing periodically if we try to open/close BLE device every time we want to read from sensors
	openBleDevice()

	for {
		err := scanAndReceive()
		if err != nil {
			log.Error("failed to scanAndReceive: %s", err)

			log.Info("attempting to reopen BLE device in 5s")
			time.Sleep(5 * time.Second)

			log.Debugf("removing all services")
			err := ble.RemoveAllServices()
			if err != nil {
				log.Errorf("failed to remove all services: %s", err)
			} else {
				log.Debugf("removed all services")
			}

			log.Debugf("stopping the device")
			err = ble.Stop()
			if err != nil {
				log.Error("failed to stop the device: %s", err)
			} else {
				log.Debugf("stopped the device")
			}

			openBleDevice()
		} else {
			watchdogChannel <- true // signal a successful read from the device
		}
		time.Sleep(*readInterval)
	}
}

func openBleDevice() {
	log.Info("Opening BLE device")
	d, err := linux.NewDevice()
	if err != nil {
		log.Panicf("failed to open ble: %s", err)
	}
	ble.SetDefaultDevice(d)
}

func scanAndReceive() error {
	log.Info("scanning...")

	// Scan
	scanner := waveplus.BleScanner{
		ScanDuration: *scanDuration,
		Retries:      *retries,
	}
	log.Debugf("scanning for sensors")
	sensorsMap, err := scanner.Scan()
	if err != nil {
		return errors.Wrap(err, "failed to scan for sensors: %s")
	}
	log.Debugf("scan finished")

	for serialNr, sensor := range sensorsMap {
		log.Printf("Found: serialNr %s addr %s", serialNr, sensor.Address())
	}

	// Receive from every found sensor
	for serialNr, sensor := range sensorsMap {
		log.Debugf("receiving sensor values from %s", serialNr)
		values, err := sensor.Receive()
		if err != nil {
			log.Errorf("failed to read from sensor (serialNr %s): %s", serialNr, err)
			continue
		}
		log.Debugf("finished receiving")

		valuesAsJson, err := json.Marshal(values)
		if err == nil {
			log.Printf("Received from %s: %s", serialNr, valuesAsJson)
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

	return nil
}
