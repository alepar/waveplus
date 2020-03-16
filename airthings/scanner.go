package airthings

type Scanner interface {

	// returns map from SerialNumber to sensor struct
	Scan() (map[string]Sensor, error)
}
