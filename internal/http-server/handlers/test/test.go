package test

import (
	grpcDevice "Dispatcher/internal/client/DeviceService/grpc"
	"errors"
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

type TestRequest struct {
	SourceID   uint `json:"source_id"`
	TestNumber uint `json:"test_number"`
}
type TestResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type Handler struct {
	testStorage TestCycleBuffer
	grpcDevice  *grpcDevice.Client
}

type TestCycleBuffer interface {
	CheckAvailableSpace() (int64, error)
	GetMaxSize() int64
	GetCurrId() int64
	SaveTest(test *TestRequest) error
	GetTrashTest() (*TrashTest, error)
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
		log.Info("availableSpace=", availableSpace)
		maxSize := handler.testStorage.GetMaxSize()
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
			//TODO: send report to UserService and StatisticService
		} else if availableSpace == maxSize {
			log.Info("Trying to send test to device, if it's imposible, try to send test to buffer")
			//TODO: send report to Devices (there is two grpc methods getFreeDevices and sendTest)
			//TODO: send report to UserService and StatisticService
		} else if availableSpace < maxSize {
			log.Info("Save test in buffer")
			handler.testStorage.SaveTest(&req)
			//TODO: send report to UserService and StatisticService
		}

		render.JSON(w, r, TestResponse{
			Message: "Data received successfully",
			Status:  "success",
		})
	}
}

func NewHandler(ts TestCycleBuffer, gd *grpcDevice.Client) *Handler {
	return &Handler{
		testStorage: ts,
		grpcDevice:  gd,
	}
}
