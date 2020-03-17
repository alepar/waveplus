package waveplus

import (
	"context"
	"fmt"
	"time"

	"github.com/go-ble/ble"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/alepar/airthings/airthings"
)

type BleScanner struct {
	ScanDuration time.Duration
	Retries      int
}

func (scanner *BleScanner) Scan() (map[string]airthings.Sensor, error) {
	var lastErr error
	var devices map[string]airthings.Sensor
	for i := 0; i < scanner.Retries; i++ {
		devices, lastErr = scanner.scan()
		if lastErr == nil {
			return devices, nil
		}
		if i < scanner.Retries {
			log.Errorf("retrying error in scan: %s", lastErr)
		}
	}

	return map[string]airthings.Sensor{}, errors.Wrap(lastErr, "all retries to scan failed")
}

func (scanner *BleScanner) scan() (map[string]airthings.Sensor, error) {
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), scanner.ScanDuration))
	ads, err := ble.Find(ctx, false, wavePlusOnlyFilter)
	if err != nil {
		switch errors.Cause(err) {
		case nil:
		case context.DeadlineExceeded:
		case context.Canceled:
			return map[string]airthings.Sensor{}, errors.Wrap(err, "scan for devices cancelled")
		default:
			return map[string]airthings.Sensor{}, errors.Wrap(err, "failed to scan for devices")
		}
	}

	sensorMap := map[string]airthings.Sensor{}

	for _, a := range ads {
		addr := a.Addr().String()
		serialNr := manufacturerDataToSerialNumber(a.ManufacturerData())
		sensorMap[serialNr] = &BleSensor{
			Addr:         addr,
			ScanDuration: scanner.ScanDuration,
			Retries:      scanner.Retries,
		}
	}

	return sensorMap, nil
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

func manufacturerDataToSerialNumber(manufacturerData []byte) string {
	serialNumber := uint32(manufacturerData[2])
	serialNumber |= uint32(manufacturerData[3]) << 8
	serialNumber |= uint32(manufacturerData[4]) << 16
	serialNumber |= uint32(manufacturerData[5]) << 24
	return fmt.Sprint(serialNumber)
}
