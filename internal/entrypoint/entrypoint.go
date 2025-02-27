package entrypoint

import (
	grpcDevice "Dispatcher/internal/client/DeviceService/grpc"
	"Dispatcher/internal/config"
	"Dispatcher/internal/http-server/handlers/test"
	"Dispatcher/internal/logger"
	storage "Dispatcher/internal/storage/postgres"
	"context"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"log"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type (
	// Entrypoint entrypoint methods.
	Entrypoint interface {
		Run() error
	}

	entrypoint struct {
		cfg           *config.Config
		logger        *slog.Logger
		kafkaProducer *kafka.Producer
		st            *storage.Storage
		router        *chi.Mux
		grpcClient    *grpcDevice.Client
		mu            sync.Mutex
		startTime     time.Time
		eventCount    int
	}
)

func New(cfg *config.Config) (Entrypoint, error) {
	ep := &entrypoint{cfg: cfg}

	var err error

	ep.logger = logger.SetupLogger(ep.cfg.Env)
	ep.logger.Info("UserService starts", slog.String("env", cfg.Env))

	// init db
	ep.st, err = storage.New(cfg, ep.logger)
	if err != nil {
		ep.logger.Error("Ошибка создания хранилища", "error", err)
		return nil, err
	}

	ep.logger.Info("Connecting to kafka")
	ep.kafkaProducer, err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": cfg.KafkaProducer.Broker})
	if err != nil {
		ep.logger.Error("Ошибка создания Kafka producer", "error", err)
		return nil, err
	}
	ep.logger.Info("Connected to kafka")

	// init gRPC client
	grpcClient, err := grpcDevice.New(
		ep.logger,
		ep.cfg.GRPCClient.Address,
	)

	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	http_handler := test.NewHandler(ep.st, grpcClient, ep.kafkaProducer, ep.cfg)
	router.Post("/test", test.New(
		ep.logger,
		http_handler,
	))

	ep.router = router

	go func(client *grpcDevice.Client) {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)

				devices, err := client.GetDeviceList(ctx)
				if err != nil {
					continue
				}
				availableSpace, err := ep.st.CheckAvailableSpace()
				if err != nil {
					ep.logger.Error(err.Error())
					continue
				}
				if availableSpace == cfg.MaxSize {
					continue
				}
				ep.logger.Debug("getting list of free devices", slog.Any("available space", availableSpace), slog.Any("num of devices", len(devices)), slog.Any("available devices", devices))
				if len(devices) > 0 {
					for _, device := range devices {
						pos, sourceNum, requestNum, err := ep.st.GetTest()
						if err != nil {
							ep.logger.Error(err.Error())
							break
						}
						if sourceNum == -1 && requestNum == -1 {
							break
						}
						ep.logger.Debug("try to send test", slog.Any("device", device))
						client.SendTest(ctx, device.DeviceId, int32(sourceNum), int32(requestNum))
						err = ep.st.DeleteTest(pos)
						if err != nil {
							log.Println(err)
						}
					}
				}

				cancel()
			}
		}
	}(grpcClient)

	ep.logger.Info("Creating was finished")

	return ep, nil
}

func (ep *entrypoint) Run() error {
	ep.logger.Info("Starting server")
	srv := &http.Server{
		Addr:         ep.cfg.HTTPServer.Address,
		Handler:      ep.router,
		ReadTimeout:  ep.cfg.HTTPServer.Timeout,
		WriteTimeout: ep.cfg.HTTPServer.Timeout,
		IdleTimeout:  ep.cfg.HTTPServer.IdleTimeout,
	}

	if err := srv.ListenAndServe(); err != nil {
		ep.logger.Error("failed to start server", slog.Any("error", err))
	}

	return nil
}
