package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-ble/ble/linux"
	"github.com/pkg/errors"

	"github.com/go-ble/ble"
)

var (
	readInterval = flag.Duration("read-int", 1*time.Minute, "time interval between sensor reads")
	scanInterval = flag.Duration("scan-int", 5*time.Minute, "time interval between scans for new sensors")
	scanDuration = flag.Duration("scan-dur", 4*time.Second, "scan duration")
)

func main() {
	flag.Parse()

	d, err := linux.NewDevice()
	if err != nil {
		log.Fatalf("can't new device : %s", err)
	}
	ble.SetDefaultDevice(d)

	fmt.Printf("Scanning for %s...", *scanDuration)
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), *scanDuration))
	ads, err := ble.Find(ctx, false, wavePlusOnlyFilter)
	chkErr(err)

	adsMap := map[string]ble.Advertisement {}

	for _, a := range ads {
		adsMap[a.Addr().String()] = a
	}

	for _, a := range adsMap {
		deviceAddr := a.Addr()
		fmt.Printf("Found Wave+, addr: %s, RSSI: %3d\n", deviceAddr, a.RSSI())

		filter := func(a ble.Advertisement) bool {
			return strings.ToUpper(a.Addr().String()) == strings.ToUpper(deviceAddr.String())
		}

		ctx = ble.WithSigHandler(context.WithTimeout(context.Background(), *scanDuration))
		cln, err := ble.Connect(ctx, filter)
		if err != nil {
			// TODO: retry connects
			log.Fatalf("can't connect : %s", err)
		}

		// Make sure we had the chance to print out the message.
		done := make(chan struct{})
		// Normally, the connection is disconnected by us after our exploration.
		// However, it can be asynchronously disconnected by the remote peripheral.
		// So we wait(detect) the disconnection in the go routine.
		go func() {
			<-cln.Disconnected()
			close(done)
		}()

		p, err := cln.DiscoverProfile(true)
		if err != nil {
			log.Fatalf("can't discover profile: %s", err)
		}

		explore(cln, p)
		cln.CancelConnection()
		<-done
	}
}

type WaveSensorValues struct {
	i0_unk uint8
	i1_humidity uint8
	i2_unk uint8
	i3_unk uint8
	i4_radonShort uint16
	i5_radonLong uint16
	i6_temperature uint16
	i7_atm_pressure uint16
	i8_co2 uint16
	i9_voc uint16
	i10_unk uint16
	i11_unk uint16
}

const WavePlusSensorServiceUuid = "b42e1c08ade711e489d3123b93f75cba"
const WavePlusSensorCharacteristicUuid = "b42e2a68ade711e489d3123b93f75cba"
func explore(cln ble.Client, p *ble.Profile) error {
	for _, s := range p.Services {
		if s.UUID.String() == WavePlusSensorServiceUuid {

			for _, c := range s.Characteristics {
				if c.UUID.String() == WavePlusSensorCharacteristicUuid {
					sensorBytes, err := cln.ReadCharacteristic(c)
					if err != nil {
						fmt.Printf("Failed to read characteristic: %s\n", err)
						continue
					}

					sensorUnpacked := WaveSensorValues{}
					buf := bytes.NewBuffer(sensorBytes)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i0_unk)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i1_humidity)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i2_unk)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i3_unk)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i4_radonShort)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i5_radonLong)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i6_temperature)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i7_atm_pressure)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i8_co2)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i9_voc)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i10_unk)
					binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i11_unk)

					fmt.Printf("Humidity: %f %%rH\n", float32(sensorUnpacked.i1_humidity)/2.0)
					fmt.Printf("Radon Short: %d Bq/m3\n", sensorUnpacked.i4_radonShort)
					fmt.Printf("Radon Long: %d Bq/m3\n", sensorUnpacked.i5_radonLong)
					fmt.Printf("Temperature: %f degC\n", float32(sensorUnpacked.i6_temperature)/100.0)
					fmt.Printf("Atm Pressure: %f hPa\n", float32(sensorUnpacked.i7_atm_pressure)/50.0)
					fmt.Printf("CO2 Level: %f ppm\n", float32(sensorUnpacked.i8_co2))
					fmt.Printf("VOC Level: %f ppb\n", float32(sensorUnpacked.i9_voc))
				}
			}
		}
	}
	return nil
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

func chkErr(err error) {
	switch errors.Cause(err) {
	case nil:
	case context.DeadlineExceeded:
		fmt.Printf("done\n")
	case context.Canceled:
		fmt.Printf("canceled\n")
	default:
		log.Fatalf(err.Error())
	}
}
