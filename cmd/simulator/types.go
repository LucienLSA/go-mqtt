package main

// MQTT 控制消息
type controlMessage struct {
	Cmd     string      `json:"cmd"`
	Param   interface{} `json:"param"`
	TraceID string      `json:"trace_id"`
	Ts      int64       `json:"ts"`
}

// MQTT 控制回执
type feedbackMessage struct {
	TraceID string `json:"trace_id"`
	Result  string `json:"result"`
	Message string `json:"message"`
	Ts      int64  `json:"ts"`
}

// 设备上报数据
type dataMessage struct {
	Temp    float64 `json:"temp"`
	Humi    float64 `json:"humi"`
	Voltage float64 `json:"voltage"`
	Status  string  `json:"status"`
	Ts      int64   `json:"ts"`
}
