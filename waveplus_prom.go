package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/alepar/airthings/airthings/waveplus"
)

var (
	readInterval = flag.Duration("read-int", 1*time.Minute, "time interval between sensor reads")
	scanDuration = flag.Duration("scan-dur", 2500*time.Millisecond, "scan duration")
	retries      = flag.Int("retries", 5, "max number of tries in case of BLE errors")
)

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

	for serialNr, sensor := range sensorsMap {
		values, err := sensor.Receive()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Serial Number: %s\n", serialNr)
		fmt.Printf("Humidity: %f %%rH\n", values.Humidity)
		fmt.Printf("Radon Short: %d Bq/m3\n", values.RadonShort)
		fmt.Printf("Radon Long: %d Bq/m3\n", values.RadonLong)
		fmt.Printf("Temperature: %f degC\n", values.Temperature)
		fmt.Printf("Atm Pressure: %f hPa\n", values.AtmPressure)
		fmt.Printf("CO2 Level: %f ppm\n", values.Co2Level)
		fmt.Printf("VOC Level: %f ppb\n", values.VocLevel)
	}
}
