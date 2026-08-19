package resourceprofile

import "fmt"

type Profile struct {
	Name         string
	GPUMemoryGiB int
	Sharing      string
	Preemptible  bool
}

var profiles = map[string]Profile{
	"protected-gpu": {Name: "protected-gpu", GPUMemoryGiB: 24, Sharing: "guaranteed", Preemptible: false},
	"shared-gpu":    {Name: "shared-gpu", GPUMemoryGiB: 8, Sharing: "shared", Preemptible: true},
	"batch-shared":  {Name: "batch-shared", GPUMemoryGiB: 8, Sharing: "shared", Preemptible: true},
}

func Resolve(name string) (Profile, error) {
	p, ok := profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown resource profile %q", name)
	}
	return p, nil
}
