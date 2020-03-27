package waveplus

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"time"

	"github.com/go-ble/ble"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/alepar/airthings/airthings"
)

type BleSensor struct {
	Addr         string
	ScanDuration time.Duration
	Retries      int
}

func (sensor *BleSensor) Address() string {
	return sensor.Addr
}

func (sensor *BleSensor) Receive() (airthings.SensorValues, error) {
	var lastErr error
	var values airthings.SensorValues
	for i := 0; i < sensor.Retries; i++ {
		values, lastErr = sensor.receive()
		if lastErr == nil {
			return values, nil
		}
		if i < sensor.Retries {
			log.Errorf("retrying error in receive: %s", lastErr)
			time.Sleep(sensor.ScanDuration) // self-pacing interval in an attempt to fix freezes
		}
	}

	return airthings.SensorValues{}, errors.Wrap(lastErr, "all retries to receive failed")
}

func (sensor *BleSensor) receive() (airthings.SensorValues, error) {
	filter := func(a ble.Advertisement) bool {
		return strings.ToUpper(a.Addr().String()) == strings.ToUpper(sensor.Addr)
	}

	log.Debugf("connecting to device")
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), sensor.ScanDuration))
	cln, err := ble.Connect(ctx, filter)
	if err != nil {
		return airthings.SensorValues{}, errors.Wrap(err, "couldn't connect to ble")
	}

	// Normally, the connection is disconnected by us after our exploration.
	// However, it can be asynchronously disconnected by the remote peripheral.
	// So we wait(detect) the disconnection in the go routine.
	done := make(chan struct{})
	go func() {
		<-cln.Disconnected()
		log.Debugf("device disconnected")
		close(done)
	}()
	defer func() {
		log.Debugf("closing connection")
		_ = cln.CancelConnection()
		<-done
	}()

	// TODO move to init
	serviceUuid, err := ble.Parse(sensorServiceUuidStr)
	if err != nil {
		log.Fatalf("could not parse service uuid: %s", err)
	}
	log.Debugf("discovering services")
	services, err := cln.DiscoverServices([]ble.UUID{serviceUuid})
	log.Debugf("finished discovering services")
	if err != nil {
		return airthings.SensorValues{}, errors.Wrap(err, "couldn't discover services")
	}
	if len(services) == 0 {
		return airthings.SensorValues{}, errors.Wrap(err, "did not find expected sensor service")
	}

	service := services[0]
	// TODO move to init
	charUuid, err := ble.Parse(sensorCharacteristicUuid)
	if err != nil {
		log.Fatalf("could not parse characteristic uuid: %s", err)
	}
	log.Debugf("discovering characteristics")
	characteristics, err := cln.DiscoverCharacteristics([]ble.UUID{charUuid}, service)
	log.Debugf("finished discovering characteristics")
	if err != nil {
		return airthings.SensorValues{}, errors.Wrap(err, "couldn't discover characteristic")
	}
	if len(characteristics) == 0 {
		return airthings.SensorValues{}, errors.Wrap(err, "did not find expected characteristic")
	}
	c := characteristics[0]

	log.Debugf("reading characteristic")
	sensorBytes, err := cln.ReadCharacteristic(c)
	log.Debugf("finished reading characteristic")
	if err != nil {
		return airthings.SensorValues{}, errors.Wrap(err, "failed to read characteristic value")
	}

	sensorUnpacked := rawSensorValues{}
	buf := bytes.NewBuffer(sensorBytes)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i0_unk)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i1_humidity)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i2_unk)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i3_unk)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i4_radonShort)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i5_radonLong)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i6_temperature)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i7_atm_pressure)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i8_co2)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i9_voc)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i10_unk)
	_ = binary.Read(buf, binary.LittleEndian, &sensorUnpacked.i11_unk)

	return refineRawValues(sensorUnpacked), nil
}

func refineRawValues(raw rawSensorValues) airthings.SensorValues {
	return airthings.SensorValues{
		Humidity:    float32(raw.i1_humidity) / 2.0,
		RadonShort:  raw.i4_radonShort,
		RadonLong:   raw.i5_radonLong,
		Temperature: float32(raw.i6_temperature) / 100.0,
		AtmPressure: float32(raw.i7_atm_pressure) / 50.0,
		Co2Level:    float32(raw.i8_co2),
		VocLevel:    float32(raw.i9_voc),
	}
}

const sensorServiceUuidStr = "b42e1c08ade711e489d3123b93f75cba"
const sensorCharacteristicUuid = "b42e2a68ade711e489d3123b93f75cba"

type rawSensorValues struct {
	i0_unk          uint8
	i1_humidity     uint8
	i2_unk          uint8
	i3_unk          uint8
	i4_radonShort   uint16
	i5_radonLong    uint16
	i6_temperature  uint16
	i7_atm_pressure uint16
	i8_co2          uint16
	i9_voc          uint16
	i10_unk         uint16
	i11_unk         uint16
}
