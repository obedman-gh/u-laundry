package service

import (
	"context"
	"errors"
	"time"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type RotatedTracking struct {
	Token     string
	ExpiresAt time.Time
	Version   int64
}

func (s *Orders) RotateTracking(ctx context.Context, actor, id string, expected int64) (RotatedTracking, error) {
	var result RotatedTracking
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		a, err := authorize(ctx, tx, actor, ChangeStatus)
		if err != nil {
			return err
		}
		r, err := tx.LockOrder(ctx, id)
		if err != nil {
			return err
		}
		if expected < 1 || expected != r.Version {
			return conflict("stale_order", "version", "Versi pesanan telah berubah.")
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		if s.runtime.Tokens == nil {
			return errors.New("Generator tracking belum tersedia")
		}
		token, err := s.runtime.Tokens.New()
		if err != nil {
			return err
		}
		hash, ok := domain.TrackingHash(token)
		if !ok {
			return errors.New("Generator tracking tidak valid")
		}
		expires := at.Add(domain.TrackingLifetime)
		if err := tx.ReplaceTracking(ctx, id, hash, expires, expected); err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, a.ID, domain.OrderTrackingRotated, domain.OrderEntity, id, at); err != nil {
			return err
		}
		result = RotatedTracking{Token: token, ExpiresAt: expires, Version: expected + 1}
		return nil
	})
	if err != nil {
		return RotatedTracking{}, err
	}
	return result, nil
}
