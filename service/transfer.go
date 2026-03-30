package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"wallet-transfer-assignment/db"
	"wallet-transfer-assignment/model"

	"github.com/google/uuid"
)

func ProcessTransfer(ctx context.Context, req model.TransferRequest) (model.TransferResponse, error) {

	//Check idempotency
	var existing []byte
	err := db.DB.QueryRow(
		"SELECT response FROM idempotency_records WHERE idempotency_key=$1",
		req.IdempotencyKey,
	).Scan(&existing)

	if err == nil {
		var resp model.TransferResponse
		json.Unmarshal(existing, &resp)
		return resp, nil
	}

	//   Begin TX
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return model.TransferResponse{}, err
	}
	defer tx.Rollback()

	transferID := uuid.New().String()

	// insert transfer
	_, err = tx.Exec(`
		INSERT INTO transfers (id, from_wallet_id, to_wallet_id, amount, status)
		VALUES ($1,$2,$3,$4,'PENDING')
	`, transferID, req.FromWalletID, req.ToWalletID, req.Amount)
	if err != nil {
		return model.TransferResponse{}, err
	}

	//Lock wallets
	var fromBalance int64
	err = tx.QueryRow(
		`SELECT balance FROM wallets WHERE id=$1 FOR UPDATE`,
		req.FromWalletID,
	).Scan(&fromBalance)

	if err == sql.ErrNoRows {
		return model.TransferResponse{}, errors.New("from wallet not found")
	}
	if err != nil {
		return model.TransferResponse{}, err
	}

	var toBalance int64
	err = tx.QueryRow(
		`SELECT balance FROM wallets WHERE id=$1 FOR UPDATE`,
		req.ToWalletID,
	).Scan(&toBalance)

	if err == sql.ErrNoRows {
		return model.TransferResponse{}, errors.New("to wallet not found")
	}
	if err != nil {
		return model.TransferResponse{}, err
	}

	//Check balance
	if fromBalance < req.Amount {
		tx.Exec("UPDATE transfers SET status='FAILED' WHERE id=$1", transferID)
		return model.TransferResponse{}, errors.New("insufficient balance")
	}

	//  update balances
	_, err = tx.Exec(`UPDATE wallets SET balance=balance-$1 WHERE id=$2`,
		req.Amount, req.FromWalletID)
	if err != nil {
		return model.TransferResponse{}, err
	}

	_, err = tx.Exec(`UPDATE wallets SET balance=balance+$1 WHERE id=$2`,
		req.Amount, req.ToWalletID)
	if err != nil {
		return model.TransferResponse{}, err
	}

	//  Ledger entries
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (wallet_id, transfer_id, type, amount)
		VALUES ($1,$2,'DEBIT',$3), ($4,$2,'CREDIT',$3)
	`, req.FromWalletID, transferID, req.Amount, req.ToWalletID)
	if err != nil {
		return model.TransferResponse{}, err
	}

	// Update transfer
	_, err = tx.Exec("UPDATE transfers SET status='PROCESSED' WHERE id=$1", transferID)
	if err != nil {
		return model.TransferResponse{}, err
	}

	resp := model.TransferResponse{
		TransferID: transferID,
		Status:     "PROCESSED",
	}

	// Store idempotency
	respJSON, _ := json.Marshal(resp)

	_, err = tx.Exec(`
		INSERT INTO idempotency_records (idempotency_key, transfer_id, response)
		VALUES ($1,$2,$3)
	`, req.IdempotencyKey, transferID, respJSON)
	if err != nil {
		return model.TransferResponse{}, err
	}

	// Commit tx
	err = tx.Commit()
	if err != nil {
		return model.TransferResponse{}, err
	}

	return resp, nil
}
