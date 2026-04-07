package main

import (
	"flag"
	"log"
	"strconv"
	"strings"
)

// Config 表示模拟器运行参数
type Config struct {
	Broker   string
	Username string
	Password string
	DeviceID string
	Interval int
	QOS      int
}

func LoadConfig() Config {
	cfg := Config{}
	flag.StringVar(&cfg.Broker, "broker", getEnv("MQTT_BROKER", "tcp://127.0.0.1:1883"), "MQTT broker address")
	flag.StringVar(&cfg.Username, "username", getEnv("MQTT_USERNAME", "neuron-gw"), "MQTT username")
	flag.StringVar(&cfg.Password, "password", getEnv("MQTT_PASSWORD", "neuron-gw-secret"), "MQTT password")
	flag.StringVar(&cfg.DeviceID, "device-id", getEnv("SIM_DEVICE_ID", ""), "device id (required)")
	flag.IntVar(&cfg.Interval, "interval", getEnvInt("SIM_REPORT_INTERVAL", 3), "report interval seconds")
	flag.IntVar(&cfg.QOS, "qos", getEnvInt("MQTT_QOS", 0), "MQTT qos 0-2")
	flag.Parse()

	if strings.TrimSpace(cfg.DeviceID) == "" {
		log.Fatal("device-id is required, use -device-id or set SIM_DEVICE_ID in .env")
	}
	if cfg.QOS < 0 || cfg.QOS > 2 {
		log.Fatal("qos must be 0, 1 or 2")
	}
	return cfg
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(getEnv(key, ""))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
