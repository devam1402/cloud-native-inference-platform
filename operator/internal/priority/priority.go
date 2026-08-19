package priority

func Calculate(tenantPriority int32, workloadClass string, override *int32) int32 {
	if override != nil {
		return *override
	}

	switch workloadClass {
	case "interactive":
		return tenantPriority
	case "batch":
		return tenantPriority / 3
	case "background":
		return tenantPriority / 10
	default:
		return tenantPriority
	}
}
