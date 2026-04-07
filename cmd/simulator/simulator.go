package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Simulator 设备模拟器
type Simulator struct {
	cfg           Config
	client        mqtt.Client
	controlTopic  string
	feedbackTopic string
	dataTopic     string
}

func NewSimulator(cfg Config) *Simulator {
	return &Simulator{
		cfg:           cfg,
		controlTopic:  fmt.Sprintf("device/%s/control", cfg.DeviceID),
		feedbackTopic: fmt.Sprintf("device/%s/feedback", cfg.DeviceID),
		dataTopic:     fmt.Sprintf("device/%s/data", cfg.DeviceID),
	}
}

func (s *Simulator) Run() {
	rand.Seed(time.Now().UnixNano())
	s.client = s.connect()

	ticker := time.NewTicker(time.Duration(s.cfg.Interval) * time.Second)
	defer ticker.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	s.publishData()
	for {
		select {
		case <-ticker.C:
			s.publishData()
		case <-sig:
			log.Println("simulator shutting down")
			s.client.Disconnect(250)
			return
		}
	}
}

func (s *Simulator) connect() mqtt.Client {
	clientID := fmt.Sprintf("sim-%s-%d", s.cfg.DeviceID, time.Now().UnixNano())
	opts := mqtt.NewClientOptions()
	opts.AddBroker(s.cfg.Broker)
	opts.SetClientID(clientID)
	opts.SetUsername(s.cfg.Username)
	opts.SetPassword(s.cfg.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("mqtt disconnected: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		tk := c.Subscribe(s.controlTopic, byte(s.cfg.QOS), func(client mqtt.Client, msg mqtt.Message) {
			s.handleControl(client, msg)
		})
		if tk.Wait() && tk.Error() != nil {
			log.Printf("subscribe control failed: %v", tk.Error())
			return
		}
		log.Printf("subscribed control topic: %s", s.controlTopic)
	})

	client := mqtt.NewClient(opts)
	if tk := client.Connect(); tk.Wait() && tk.Error() != nil {
		log.Fatalf("mqtt connect failed: %v", tk.Error())
	}
	log.Printf("simulator connected broker=%s device_id=%s", s.cfg.Broker, s.cfg.DeviceID)
	return client
}

func (s *Simulator) publishData() {
	payload := dataMessage{
		Temp:    22 + rand.Float64()*8,
		Humi:    40 + rand.Float64()*20,
		Voltage: 3.6 + rand.Float64()*0.4,
		Status:  "running",
		Ts:      time.Now().Unix(),
	}
	body, _ := json.Marshal(payload)
	tk := s.client.Publish(s.dataTopic, byte(s.cfg.QOS), false, body)
	if tk.Wait() && tk.Error() != nil {
		log.Printf("publish data failed: %v", tk.Error())
		return
	}
	log.Printf("published data topic=%s payload=%s", s.dataTopic, string(body))
}

func (s *Simulator) handleControl(client mqtt.Client, msg mqtt.Message) {
	log.Printf("received control topic=%s payload=%s", msg.Topic(), string(msg.Payload()))

	var ctrl controlMessage
	if err := json.Unmarshal(msg.Payload(), &ctrl); err != nil {
		log.Printf("invalid control payload: %v", err)
		return
	}

	result := feedbackMessage{
		TraceID: ctrl.TraceID,
		Result:  "ok",
		Message: fmt.Sprintf("command %s applied", ctrl.Cmd),
		Ts:      time.Now().Unix(),
	}
	if strings.TrimSpace(ctrl.TraceID) == "" {
		result.Result = "fail"
		result.Message = "trace_id missing"
	}

	body, _ := json.Marshal(result)
	tk := client.Publish(s.feedbackTopic, byte(s.cfg.QOS), false, body)
	if tk.Wait() && tk.Error() != nil {
		log.Printf("publish feedback failed: %v", tk.Error())
		return
	}
	log.Printf("published feedback topic=%s payload=%s", s.feedbackTopic, string(body))
}
