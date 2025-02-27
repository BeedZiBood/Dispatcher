package httpclient

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
)

type Response struct {
	TestNum int32  `json:"testNum"`
	Result  bool   `json:"result"`
	Msg     string `json:"msg"`
}

func SendResponce(response Response, log *slog.Logger) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Error("Serialization error", "error", err)
		return
	}

	resp, err := http.Post("http://localhost:8081/", "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Error("POST error", "error", err)
		return
	}
	defer resp.Body.Close()
}
