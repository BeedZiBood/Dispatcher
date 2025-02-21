package storage

import (
	"Dispatcher/internal/config"
	"Dispatcher/internal/http-server/handlers/test"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq"
)

type Storage struct {
	db      *sql.DB
	log     *slog.Logger
	maxSize int
	currId  int
}

func New(cfg *config.Config, log *slog.Logger) (*Storage, error) {
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.PostgresConfig.Host,
		cfg.PostgresConfig.Port,
		cfg.PostgresConfig.Username,
		cfg.PostgresConfig.Password,
		cfg.PostgresConfig.DBName,
		cfg.PostgresConfig.SSLMode))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "failed to open database connection", err)
	}

	stmt, err := db.Prepare("CREATE TABLE IF NOT EXISTS circular_buffer (pos integer PRIMARY KEY, source_number integer NOT NULL, request_number integer NOT NULL, arrival_time  timestamp NOT NULL DEFAULT now());")

	if err != nil {
		return nil, fmt.Errorf("%s: %w", "Can't prepare a circular_buffer table creation", err)
	}

	_, err = stmt.Exec()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "Can't create a circular_buffer table", err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS trash_table (" +
		"source_number integer NOT NULL," +
		"request_number integer NOT NULL," +
		"arrival_time timestamp NOT NULL," +
		"removal_time timestamp NOT NULL," +
		"taken boolean NOT NULL DEFAULT false);")

	if err != nil {
		return nil, fmt.Errorf("%s: %w", "Can't prepare a trash_table table creation", err)
	}

	_, err = stmt.Exec()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", "Can't create a trash_table table", err)
	}

	return &Storage{db: db, log: log, maxSize: cfg.CycleBufferConfig.MaxSize, currId: 1}, nil
}

func (st *Storage) CheckAvailableSpace() (int, error) {
	stmt, err := st.db.Prepare("SELECT COUNT(*) FROM circular_buffer;")
	if err != nil {
		return 0, fmt.Errorf("Can't prepare a query to check available space: %w", err)
	}
	defer stmt.Close()

	var count int
	err = stmt.QueryRow().Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("Can't check available space: %w", err)
	}

	return count, nil
}

func (st *Storage) GetMaxSize() int {
	return st.maxSize
}

func (st *Storage) GetCurrId() int {
	return st.currId
}

func (st *Storage) SaveTest(test *test.TestRequest) error {
	//TODO: add logic of circular buffer
	stmt, err := st.db.Prepare("INSERT INTO circular_buffer (source_number, request_number) VALUES ($1, $2);")
	if err != nil {
		return fmt.Errorf("Can't prepare a query to save test: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(test.SourceNumber, test.TestNumber)
	if err != nil {
		return fmt.Errorf("Can't save test: %w", err)
	}

	return nil
}

func (st *Storage) GetTrashTest() (*test.TestRequest, time.Time, time.Time) {
	var (
		arrivalTime time.Time
		removalTime time.Time
	)
	data := test.TestRequest{}
	err := st.db.QueryRow("SELECT source_number, request_number, arrival_time FROM circular_buffer WHERE taken = false;").Scan(&data.SourceNumber, &data.TestNumber, &arrivalTime, &removalTime)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, time.Time{}
	}
	return &data, arrivalTime, removalTime
}
