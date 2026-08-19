package capacityenvelope

type Envelope struct {
	MaxQPS    float64
	Available bool
}

func Check(model, profileName string) (Envelope, error) {
	return Envelope{MaxQPS: 0, Available: true}, nil
}
