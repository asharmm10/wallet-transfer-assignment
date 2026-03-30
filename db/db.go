package db

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
	"os"
)

var DB *sql.DB

func InitDB() {
	connStr := os.Getenv("DB_URL")

	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatal(err)
	}
}

func InitSchema() {
	query := `
	CREATE TABLE IF NOT EXISTS wallets (
		id TEXT PRIMARY KEY,
		balance BIGINT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS transfers (
		id TEXT PRIMARY KEY,
		from_wallet_id TEXT,
		to_wallet_id TEXT,
		amount BIGINT,
		status TEXT,
		created_at TIMESTAMP DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS ledger_entries (
		id SERIAL PRIMARY KEY,
		wallet_id TEXT,
		transfer_id TEXT,
		type TEXT,
		amount BIGINT,
		created_at TIMESTAMP DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS idempotency_records (
		idempotency_key TEXT PRIMARY KEY,
		transfer_id TEXT,
		response JSONB,
		created_at TIMESTAMP DEFAULT now()
	);
	`

	_, err := DB.Exec(query)
	if err != nil {
		log.Fatal(err)
	}

	DB.Exec(`
		INSERT INTO wallets (id, balance)
		VALUES ('wallet_1', 1000), ('wallet_2', 500)
		ON CONFLICT (id) DO NOTHING;
	`)
}
