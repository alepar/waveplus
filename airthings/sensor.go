package airthings

type Sensor interface {
	Address() string
	SerialNumber() string
	Receive() SensorValues
}

type SensorValues struct {
	// units: % of relative Humidity
	Humidity float32

	// units: Bq/m3
	RadonShort uint16

	// units: Bq/m3
	RadonLong uint16

	// units: degrees Celsius
	Temperature float32

	// units: hPa
	AtmPressure float32

	// units: ppm
	Co2Level float32

	// units: ppb
	VocLevel float32
}
