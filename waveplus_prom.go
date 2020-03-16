package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux"
	"github.com/pkg/errors"

	"github.com/alepar/airthings/airthings/waveplus"
)

var (
	readInterval = flag.Duration("read-int", 1*time.Minute, "time interval between sensor reads")
	scanInterval = flag.Duration("scan-int", 5*time.Minute, "time interval between scans for new sensors")
	scanDuration = flag.Duration("scan-dur", 5*time.Second, "scan duration")
)

func main() {
	flag.Parse()

	d, err := linux.NewDevice()
	if err != nil {
		panic(errors.Wrap(err, "failed to open ble"))
	}
	ble.SetDefaultDevice(d)

	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *scanDuration))
	ads, err := ble.Find(ctx, false, wavePlusOnlyFilter)
	if err != nil {
		switch errors.Cause(err) {
		case nil:
		case context.DeadlineExceeded:
		case context.Canceled:
			panic(errors.Wrap(err, "scan for devices cancelled"))
		default:
			panic(errors.Wrap(err, "failed to scan for devices"))
		}
	}

	adsMap := map[string]ble.Advertisement{}

	for _, a := range ads {
		adsMap[a.Addr().String()] = a
	}

	for _, a := range adsMap {
		sensor := waveplus.NewBleSensor(
			a.Addr().String(),
			a.ManufacturerData(),
			*scanDuration,
			uint8(5),
		)

		values, err := sensor.Receive()
		if err != nil {
			panic(err)
		}

		fmt.Printf("Humidity: %f %%rH\n", values.Humidity)
		fmt.Printf("Radon Short: %d Bq/m3\n", values.RadonShort)
		fmt.Printf("Radon Long: %d Bq/m3\n", values.RadonLong)
		fmt.Printf("Temperature: %f degC\n", values.Temperature)
		fmt.Printf("Atm Pressure: %f hPa\n", values.AtmPressure)
		fmt.Printf("CO2 Level: %f ppm\n", values.Co2Level)
		fmt.Printf("VOC Level: %f ppb\n", values.VocLevel)
	}
}

func wavePlusOnlyFilter(a ble.Advertisement) bool {
	if a.Connectable() {
		manufacturerData := a.ManufacturerData()
		if len(manufacturerData) >= 6 && manufacturerData[0] == 0x34 && manufacturerData[1] == 0x03 {
			return true
		}
	}

	return false
}
