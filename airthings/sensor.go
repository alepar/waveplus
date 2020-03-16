package airthings

type Sensor interface {
	Address() string
	SerialNumber() string
	Receive() SensorValues
}

type SensorValues struct {
	// units: % of relative Humidity
	humidity float32

	// units: Bq/m3
	radonShort uint16

	// units: Bq/m3
	radonLong uint16

	// units: degrees Celsius
	temperature float32

	// units: hPa
	atmPressure float32

	// units: ppm
	co2Level float32

	// units: ppb
	vocLevel float32
}
