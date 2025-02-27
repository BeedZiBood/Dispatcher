package test

import (
	grpcDevice "Dispatcher/internal/client/DeviceService/grpc"
	"Dispatcher/internal/config"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
)

type TrashTest struct {
	TestRequest
	ArrivalTime time.Time
	RemovalTime time.Time
}

type UserServiceTestResponse struct {
	TestReq TestRequest `json:"test_req"`
	Status  bool        `json:"status"`
}
type KafkaData struct {
	AvailableSpace int64 `json:"availableSpace"`
	MaxSize        int64 `json:"maxSize"`
}

type TestRequest struct {
	SourceID   uint `json:"source_id"`
	TestNumber uint `json:"test_number"`
}
type TestResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type Handler struct {
	testStorage   TestCycleBuffer
	grpcDevice    *grpcDevice.Client
	kafkaProducer *kafka.Producer
	Cfg           *config.Config
}

type TestCycleBuffer interface {
	CheckAvailableSpace() (int64, error)
	GetMaxSize() int64
	GetCurrId() int64
	SaveTest(test *TestRequest) error
	GetTrashTest() (*TrashTest, error)
	GetTest() (int64, int64, int64, error)
	DeleteTest(pos int64) error
}

func New(log *slog.Logger, handler *Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.test.New"

		log := log.With(
			slog.String("op", op),
		)

		var req TestRequest
		err := render.DecodeJSON(r.Body, &req)
		if errors.Is(err, io.EOF) {
			log.Error("request body is empty")

			render.JSON(w, r, TestResponse{
				Message: "Request body is empty",
				Status:  "error",
			})

			return
		}
		if err != nil {
			log.Error("failed to decode request body", err)

			render.JSON(w, r, TestResponse{
				Message: "Failed to decode request body",
				Status:  "error",
			})

			return
		}

		log.Info("request body decoded", slog.Any("request", req))

		availableSpace, err := handler.testStorage.CheckAvailableSpace()
		if err != nil {
			log.Error("failed to check available space", err)

			render.JSON(w, r, TestResponse{
				Message: "Failed to check available space",
				Status:  "error",
			})

			return
		}
		log.Info("check available space", slog.Any("available space", availableSpace))
		maxSize := handler.testStorage.GetMaxSize()
		data := KafkaData{
			AvailableSpace: availableSpace,
			MaxSize:        maxSize,
		}
		if availableSpace == 0 {
			log.Info("Send test to buffer and get trash test from trash_table")
			err := handler.testStorage.SaveTest(&req)
			if err != nil {
				log.Error("failed to save test", err)
			}
			trashTest, err := handler.testStorage.GetTrashTest()
			if err != nil {
				log.Error("failed to get trash test", err)
			}
			log.Info("trash test", slog.Any("test", trashTest))
			response := UserServiceTestResponse{
				TestReq: req,
				Status:  false,
			}
			sendTest(&response, log)
			sendToKafka(&data, handler, log)
		} else if availableSpace == maxSize {
			log.Info("Trying to send test to device, if it's imposible, try to send test to buffer")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)

			devices, err := handler.grpcDevice.GetDeviceList(ctx)
			if err != nil {
				log.Error(err.Error())
				log.Info("Save test in buffer")
				handler.testStorage.SaveTest(&req)
			} else {
				log.Debug("getting list of free devices", slog.Any("num of devices", len(devices)), slog.Any("available devices", devices))
				if len(devices) > 0 {
					log.Debug("try to send test", slog.Any("device", devices[0]))
					handler.grpcDevice.SendTest(ctx, devices[0].DeviceId, int32(req.SourceID), int32(req.TestNumber))
				} else {
					err := handler.testStorage.SaveTest(&req)
					if err != nil {
						log.Error("failed to save test", err)
					}
				}

				cancel()

				data.AvailableSpace--
				sendToKafka(&data, handler, log)
			}
		} else if availableSpace < maxSize {
			log.Info("Save test in buffer")
			handler.testStorage.SaveTest(&req)
		}

		render.JSON(w, r, TestResponse{
			Message: "Data received successfully",
			Status:  "success",
		})
	}
}

// sendToKafka сериализует данные теста и отправляет их в Kafka.
func sendToKafka(kafkaData *KafkaData, handler *Handler, log *slog.Logger) {
	data, err := json.Marshal(kafkaData)
	if err != nil {
		slog.Error("Ошибка сериализации в sendToKafka", "error", err)
		return
	}

	err = handler.kafkaProducer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &handler.Cfg.KafkaProducer.Topic, Partition: kafka.PartitionAny},
		Value:          data,
	}, nil)

	if err != nil {
		log.Error("Ошибка отправки в Kafka", "error", err)
	} else {
		log.Info("Данные успешно отправлены в Kafka", "topic", handler.Cfg.KafkaProducer.Topic)
	}
}

func sendTest(test *UserServiceTestResponse, log *slog.Logger) {
	data, err := json.Marshal(test)
	if err != nil {
		log.Error("Ошибка сериализации теста", "error", err)
		return
	}

	resp, err := http.Post("http://localhost:8081/test", "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Error("Ошибка отправки POST запроса", "error", err)
		return
	}
	defer resp.Body.Close()

	log.Info("Тест успешно отправлен", "status", resp.Status)
}

func NewHandler(ts TestCycleBuffer, gd *grpcDevice.Client, kafkaProducer *kafka.Producer, cfg *config.Config) *Handler {
	return &Handler{
		testStorage:   ts,
		grpcDevice:    gd,
		kafkaProducer: kafkaProducer,
		Cfg:           cfg,
	}
}
