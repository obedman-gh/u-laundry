package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

func commandFingerprint(value any) ([32]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}
func prepareOrderCommand(ctx context.Context, tx repository.Tx, actor, key, operation string, hash [32]byte) (*repository.OrderCommandReceipt, error) {
	if err := domain.ValidateIdempotencyKey(key); err != nil {
		return nil, err
	}
	if err := tx.LockOrderCommandKey(ctx, actor, key); err != nil {
		return nil, err
	}
	r, err := tx.FindOrderCommand(ctx, actor, key)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.Operation != operation || r.PayloadHash != hash {
		return nil, conflict("idempotency_conflict", "idempotency_key", "Kunci perintah telah dipakai untuk data berbeda.")
	}
	return &r, nil
}
func replayOrderCommand(ctx context.Context, tx repository.Tx, r repository.OrderCommandReceipt) (repository.OrderRecord, error) {
	current, err := tx.LockOrder(ctx, r.OrderID)
	if err != nil {
		return repository.OrderRecord{}, err
	}
	if current.Version < r.Version {
		return repository.OrderRecord{}, &repository.Error{Kind: repository.InvalidData, Operation: "replay order command"}
	}
	current.Replayed = true
	current.AppliedVersion = r.Version
	return current, nil
}

func (s *Orders) History(ctx context.Context, actor, id string, page repository.Page) ([]domain.OrderStatusEvent, error) {
	var result []domain.OrderStatusEvent
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadOrders); err != nil {
			return err
		}
		if _, err := r.GetOrder(ctx, id); err != nil {
			return err
		}
		var err error
		result, err = r.OrderHistory(ctx, id, page)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
