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

type TestRequest struct {
	SourceNumber uint `json:"id"`
	TestNumber   uint `json:"test_number"`
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
	CheckAvailableSpace() (int, error)
	GetMaxSize() int
	GetCurrId() int
	SaveTest(test *TestRequest) error
	GetTrashTest() (*TestRequest, time.Time, time.Time)
}

func New(log *slog.Logger, handler *Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		maxSize := handler.testStorage.GetMaxSize()
		if availableSpace == 0 {
			handler.testStorage.SaveTest(&req)
			trashTest, arrivalTime, removalTime := handler.testStorage.GetTrashTest()
			log.Info("trash test", slog.Any("test", trashTest), slog.Any("arrival_time", arrivalTime), slog.Any("removal_time", removalTime))
			//TODO: send report to UserService and StatisticService
		} else if availableSpace == maxSize {
			//TODO: send report to Devices (there is two grpc methods getFreeDevices and sendTest)
			//TODO: send report to UserService and StatisticService
		} else if availableSpace < maxSize {
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
