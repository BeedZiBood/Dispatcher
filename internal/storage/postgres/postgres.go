package storage

import (
	"Dispatcher/internal/config"
	"Dispatcher/internal/http-server/handlers/test"
	"context"
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
	maxSize int64
	currId  int64
}

func New(cfg *config.Config, log *slog.Logger) (*Storage, error) {
	const op = "storage.postgres.New"
	log = log.With(slog.String("op", op))
	log.Info("connecting to db")
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

	stmt, err := db.Prepare("CREATE TABLE IF NOT EXISTS circular_buffer (" +
		"pos integer PRIMARY KEY, source_number integer NOT NULL," +
		"request_number integer NOT NULL," +
		"arrival_time timestamp NOT NULL DEFAULT now());")

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

	log.Info("successfully connected to db")

	return &Storage{db: db, log: log, maxSize: cfg.CycleBufferConfig.MaxSize, currId: 1}, nil
}

func (st *Storage) CheckAvailableSpace() (int64, error) {
	stmt, err := st.db.Prepare("SELECT COUNT(*) FROM circular_buffer;")
	if err != nil {
		return 0, fmt.Errorf("Can't prepare a query to check available space: %w", err)
	}
	defer stmt.Close()

	var count int64
	err = stmt.QueryRow().Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("Can't check available space: %w", err)
	}

	return st.maxSize - count, nil
}

func (st *Storage) GetMaxSize() int64 {
	return st.maxSize
}

func (st *Storage) GetCurrId() int64 {
	return st.currId
}

func (st *Storage) SaveTest(test *test.TestRequest) error {
	const op = "storage.postgres.SaveTest"
	log := st.log.With(slog.String("op", op))
	pos, err := st.db.Prepare("SELECT pos FROM circular_buffer;")

	positions := make(map[int64]bool, st.maxSize)
	rows, err := pos.Query()
	if err != nil {
		fmt.Errorf("%s: %w", "Can't prepare a query", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			fmt.Errorf("Can't get value: %w", err)
		}
		positions[p] = true
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("row iteration error: %v", err)
	}

	if len(positions) > 0 {
		var counter int64 = 0
		for counter <= st.maxSize {
			st.log.Debug("Current position:", slog.Any("pos", st.currId), slog.Any("value", positions[st.currId]))
			if st.currId == st.maxSize {
				st.currId = 0
			}
			if !positions[st.currId] {
				break
			}
			counter++
			st.currId++
		}
		st.log.Debug("Checking counter", slog.Any("counter", counter))
		if counter > st.maxSize {
			st.log.Debug("Moving test to trash table")
			err := st.moveToTrash()
			if err != nil {
				fmt.Errorf("%s: %w", "Can't move to trash", err)
			}
		}
	}

	stmt, err := st.db.Prepare("INSERT INTO circular_buffer (pos, source_number, request_number) VALUES ($1, $2, $3);")
	if err != nil {
		return fmt.Errorf("Can't prepare a query to save test: %w", err)
	}
	defer stmt.Close()

	log.Info("Saving test",
		"source_number", test.SourceID,
		"test_number", test.TestNumber,
	)

	_, err = stmt.Exec(st.currId, test.SourceID, test.TestNumber)
	if err != nil {
		return fmt.Errorf("Can't save test: %w", err)
	}

	log.Info("Test saved")

	return nil
}

func (st *Storage) GetTrashTest() (*test.TrashTest, error) {
	const op = "storage.postgres.GetTrashTest"

	log := st.log.With(slog.String("op", op))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer tx.Rollback()

	data := test.TrashTest{}
	log.Info("Trying to delete taken rows")
	_, err = tx.ExecContext(ctx, `DELETE FROM trash_table 
         WHERE taken = true`,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	log.Info("Taken rows deleted")

	log.Info("Trying to take new trash rows")
	err = tx.QueryRowContext(ctx,
		"SELECT source_number, request_number, arrival_time, removal_time FROM trash_table WHERE taken = false;",
	).Scan(&data.SourceID, &data.TestNumber, &data.ArrivalTime, &data.RemovalTime)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}

	log.Info("Trash data was taken")

	_, err = tx.ExecContext(ctx,
		`UPDATE trash_table 
         SET taken = true 
         WHERE taken = false`)
	if err != nil {
		st.log.Error("update error: ", err)
	}

	if err = tx.Commit(); err != nil {
		st.log.Error("commit error: ", err)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &data, nil
}

func (st *Storage) moveToTrash() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction error: %v", err)
	}
	defer tx.Rollback()

	var (
		currentID     int64
		sourceNumber  int
		requestNumber int
		arrivalTime   time.Time
	)

	err = tx.QueryRowContext(ctx,
		`SELECT pos, source_number, request_number, arrival_time 
         FROM circular_buffer 
         ORDER BY source_number, request_number LIMIT 1`,
	).Scan(&currentID, &sourceNumber, &requestNumber, &arrivalTime)

	if err != nil {
		return fmt.Errorf("select error: %v", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO trash_table 
         (source_number, request_number, arrival_time, removal_time) 
         VALUES ($1, $2, $3, NOW())`,
		sourceNumber, requestNumber, arrivalTime,
	)
	if err != nil {
		return fmt.Errorf("insert error: %v", err)
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM circular_buffer 
         WHERE pos = $1`,
		currentID,
	)
	if err != nil {
		return fmt.Errorf("delete error: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit error: %v", err)
	}

	st.currId = currentID
	return nil
}

func (st *Storage) GetTest() (int64, int64, error) {
	query := `
        SELECT source_number, request_number 
        FROM circular_buffer 
        ORDER BY source_number, request_number 
        LIMIT 1`

	var source, request int64

	// Выполняем запрос и сканируем результат
	err := st.db.QueryRow(query).Scan(&source, &request)
	if err != nil {
		if err == sql.ErrNoRows {
			return -1, -1, nil
		}
		return 0, 0, fmt.Errorf("error querying record: %v", err)
	}

	return source, request, nil
}
