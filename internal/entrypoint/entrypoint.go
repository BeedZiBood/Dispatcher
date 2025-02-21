package entrypoint

import (
	grpcDevice "Dispatcher/internal/client/DeviceService/grpc"
	"Dispatcher/internal/config"
	"Dispatcher/internal/http-server/handlers/test"
	"Dispatcher/internal/logger"
	storage "Dispatcher/internal/storage/postgres"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
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

	ep.kafkaProducer, err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": cfg.KafkaProducer.Broker})
	if err != nil {
		ep.logger.Error("Ошибка создания Kafka producer", "error", err)
		return nil, err
	}

	// init gRPC client
	grpcClient, err := grpcDevice.New(
		ep.logger,
		ep.cfg.GRPCClient.Address,
	)

	_ = grpcClient

	//TODO: Init gRPC server

	//TODO: Init HTTP server
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	http_handler := test.NewHandler(ep.st, grpcClient)
	router.Post("/test", test.New(
		ep.logger,
		http_handler,
	))

	ep.router = router

	ep.logger.Info("Creating was finished")

	return ep, nil
}

func (ep *entrypoint) Run() error {
	srv := &http.Server{
		Addr:         ep.cfg.HTTPServer.Address,
		Handler:      ep.router,
		ReadTimeout:  ep.cfg.HTTPServer.Timeout,
		WriteTimeout: ep.cfg.HTTPServer.Timeout,
		IdleTimeout:  ep.cfg.HTTPServer.IdleTimeout,
	}

	if err := srv.ListenAndServe(); err != nil {
		ep.logger.Error("failed to start server")
	}

	return nil
}
