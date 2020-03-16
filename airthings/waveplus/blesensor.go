package waveplus

import (
	"fmt"

	"github.com/alepar/airthings/airthings"
)

type BleSensor struct {
	address string
	manufacturerData []byte
}

func (sensor *BleSensor) Address() string {
	return sensor.address
}

func (sensor *BleSensor) SerialNumber() string {
	return ManufacturerDataToSerialNumber(sensor.manufacturerData)
}

func ManufacturerDataToSerialNumber(manufacturerData []byte) string {
	serialNumber := uint32(manufacturerData[2])
	serialNumber |= uint32(manufacturerData[3]) << 8
	serialNumber |= uint32(manufacturerData[4]) << 16
	serialNumber |= uint32(manufacturerData[5]) << 24
	return fmt.Sprint(serialNumber)
}

func (sensor *BleSensor) Receive() airthings.SensorValues {

}