package config

type SonicConfig struct {
	Host     string
	Port     int
	Password string
}

func LoadSonicConfig() *SonicConfig {
	portStr := getEnv("SONIC_PORT", "1491")
	port := 1491
	if p, err := parseInt(portStr); err == nil {
		port = p
	}
	return &SonicConfig{
		Host:     getEnv("SONIC_HOST", "localhost"),
		Port:     port,
		Password: getEnv("SONIC_PASSWORD", "SecretPassword"),
	}
}
