package service

import (
	"context"
	"strings"
	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
)

type PasswordHasher interface{ Hash(string) (string, error) }
type StaffManagement struct {
	uow       repository.UnitOfWork
	runtime   Runtime
	passwords PasswordHasher
}

func NewStaffManagement(uow repository.UnitOfWork, rt Runtime, passwords PasswordHasher) *StaffManagement {
	return &StaffManagement{uow, rt, passwords}
}

type StaffInput struct {
	Username, Name, Password string
	Role                     domain.Role
}
type StaffChange struct {
	Version  int64
	Role     domain.Role
	Active   bool
	Password string
}

func (s *StaffManagement) Create(ctx context.Context, actor string, in StaffInput) (repository.StaffRecord, error) {
	return s.create(ctx, actor, in, false)
}

// Bootstrap is command-only: no existing domain staff, no recovery/reset bypass.
func (s *StaffManagement) Bootstrap(ctx context.Context, in StaffInput) (repository.StaffRecord, error) {
	in.Role = domain.Owner
	return s.create(ctx, "", in, true)
}
func (s *StaffManagement) create(ctx context.Context, actor string, in StaffInput, bootstrap bool) (repository.StaffRecord, error) {
	var result repository.StaffRecord
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.LockStaffManagement(ctx); err != nil {
			return err
		}
		if bootstrap {
			n, err := tx.CountStaff(ctx, false)
			if err != nil {
				return err
			}
			if n != 0 {
				return conflict("bootstrap_closed", "staff", "Bootstrap hanya untuk tabel staf kosong.")
			}
		} else {
			if _, err := authorize(ctx, tx, actor, ManageStaff); err != nil {
				return err
			}
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		id, err := s.runtime.id()
		if err != nil {
			return err
		}
		v := domain.Staff{ID: id, Username: in.Username, Name: strings.TrimSpace(in.Name), Role: in.Role, Active: true, CreatedAt: at, UpdatedAt: at}
		if err := v.Validate(); err != nil {
			return err
		}
		if err := domain.ValidateNewPassword(in.Password); err != nil {
			return err
		}
		hash, err := s.passwords.Hash(in.Password)
		if err != nil {
			return err
		}
		if err := tx.AddStaff(ctx, v, hash); err != nil {
			return err
		}
		action := domain.StaffCreated
		if bootstrap {
			actor = id
			action = domain.StaffBootstrapped
		}
		if err := audit(ctx, tx, s.runtime, actor, action, domain.StaffEntity, id, at); err != nil {
			return err
		}
		result = repository.StaffRecord{Staff: v, Version: 1}
		return nil
	})
	if err != nil {
		return repository.StaffRecord{}, err
	}
	return result, nil
}
func (s *StaffManagement) List(ctx context.Context, actor string, page repository.Page) ([]repository.StaffRecord, error) {
	var result []repository.StaffRecord
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ManageStaff); err != nil {
			return err
		}
		var err error
		result, err = r.ListStaff(ctx, page)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *StaffManagement) Change(ctx context.Context, actor, id, operation string, in StaffChange) (repository.StaffRecord, error) {
	var result repository.StaffRecord
	err := s.uow.WithinTransaction(ctx, func(tx repository.Tx) error {
		if err := tx.LockStaffManagement(ctx); err != nil {
			return err
		}
		if _, err := authorize(ctx, tx, actor, ManageStaff); err != nil {
			return err
		}
		v, err := tx.LockStaff(ctx, id)
		if err != nil {
			return err
		}
		if in.Version < 1 || in.Version != v.Version {
			return conflict("stale_staff", "version", "Staf sudah berubah. Muat ulang sebelum menyimpan.")
		}
		at, err := s.runtime.now()
		if err != nil {
			return err
		}
		if at.Before(v.Staff.UpdatedAt) {
			return invalid("clock", "Waktu pembaruan tidak valid.")
		}
		action := domain.StaffUpdated
		hash := ""
		switch operation {
		case "active":
			if actor == id && !in.Active {
				return conflict("self_lockout", "staff", "Owner tidak dapat menonaktifkan akun sendiri.")
			}
			v.Staff.Active = in.Active
			action = domain.StaffDeactivated
			if in.Active {
				action = domain.StaffActivated
			}
		case "role":
			if actor == id && in.Role != domain.Owner {
				return conflict("self_lockout", "staff", "Owner tidak dapat menurunkan role sendiri.")
			}
			v.Staff.Role = in.Role
			action = domain.StaffRoleChanged
		case "password":
			if err := domain.ValidateNewPassword(in.Password); err != nil {
				return err
			}
			hash, err = s.passwords.Hash(in.Password)
			if err != nil {
				return err
			}
			action = domain.StaffPasswordReset
		default:
			return invalid("operation", "Perubahan staf tidak dikenal.")
		}
		if !v.Staff.Active || v.Staff.Role != domain.Owner {
			old, err := tx.GetStaff(ctx, id)
			if err != nil {
				return err
			}
			if old.Active && old.Role == domain.Owner {
				n, err := tx.CountStaff(ctx, true)
				if err != nil {
					return err
				}
				if n <= 1 {
					return conflict("last_owner", "staff", "Minimal satu owner aktif harus dipertahankan.")
				}
			}
		}
		v.Staff.UpdatedAt = at
		if err := v.Staff.Validate(); err != nil {
			return err
		}
		if err := tx.UpdateStaff(ctx, v.Staff, v.Version, hash); err != nil {
			return err
		}
		if err := audit(ctx, tx, s.runtime, actor, action, domain.StaffEntity, id, at); err != nil {
			return err
		}
		v.Version++
		result = v
		return nil
	})
	if err != nil {
		return repository.StaffRecord{}, err
	}
	return result, nil
}
func (s *StaffManagement) Audit(ctx context.Context, actor string, entity domain.AuditEntity, id string, page repository.Page) ([]domain.AuditEvent, error) {
	var result []domain.AuditEvent
	err := s.uow.View(ctx, func(r repository.Readers) error {
		if _, err := authorize(ctx, r, actor, ReadAudit); err != nil {
			return err
		}
		var err error
		result, err = r.AuditForEntity(ctx, entity, id, page)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
