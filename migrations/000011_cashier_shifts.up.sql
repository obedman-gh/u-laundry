-- Historical ledger entries remain unassigned. Opening/closing never create revenue.
CREATE TABLE public.cash_shifts (
 id VARCHAR(100) PRIMARY KEY,
 cashier_id VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
 open_key VARCHAR(100) NOT NULL CHECK(open_key ~ '^[A-Za-z0-9_.-]{1,100}$'),
 opening BIGINT NOT NULL CHECK(opening>=0),
 opening_notes VARCHAR(250) NOT NULL,
 opened_at TIMESTAMPTZ NOT NULL CHECK(isfinite(opened_at)),
 closed_at TIMESTAMPTZ CHECK(isfinite(closed_at) AND closed_at>=opened_at),
 closed_by VARCHAR(100) REFERENCES public.staff(id) ON DELETE RESTRICT,
 close_key VARCHAR(100) CHECK(close_key ~ '^[A-Za-z0-9_.-]{1,100}$'),
 system_cash BIGINT, physical_cash BIGINT, difference BIGINT,
 closing_notes VARCHAR(250) NOT NULL DEFAULT '',
 UNIQUE(cashier_id,open_key),
 CHECK((closed_at IS NULL AND closed_by IS NULL AND close_key IS NULL AND system_cash IS NULL AND physical_cash IS NULL AND difference IS NULL AND closing_notes='')
 OR(closed_at IS NOT NULL AND closed_by IS NOT NULL AND closed_by=cashier_id AND close_key IS NOT NULL
 AND system_cash IS NOT NULL AND system_cash>=0 AND physical_cash IS NOT NULL AND physical_cash>=0
 AND difference IS NOT NULL AND difference=physical_cash-system_cash AND (difference=0 OR length(btrim(closing_notes))>0)))
);
CREATE UNIQUE INDEX cash_shifts_one_active ON public.cash_shifts(cashier_id) WHERE closed_at IS NULL;
CREATE INDEX cash_shifts_opened_idx ON public.cash_shifts(opened_at,id);
ALTER TABLE public.ledger_entries ADD COLUMN shift_id VARCHAR(100) REFERENCES public.cash_shifts(id) ON DELETE RESTRICT;
CREATE INDEX ledger_shift_time_idx ON public.ledger_entries(shift_id,occurred_at,id) WHERE shift_id IS NOT NULL;
ALTER TABLE public.ledger_entries ADD CONSTRAINT ledger_shift_identity UNIQUE(id,shift_id);
CREATE TABLE public.shift_cash_movements (
 ledger_id VARCHAR(100) PRIMARY KEY,
 shift_id VARCHAR(100) NOT NULL REFERENCES public.cash_shifts(id) ON DELETE RESTRICT,
 idempotency_key VARCHAR(100) NOT NULL CHECK(idempotency_key ~ '^[A-Za-z0-9_.-]{1,100}$'),
 UNIQUE(shift_id,idempotency_key),
 FOREIGN KEY(ledger_id,shift_id) REFERENCES public.ledger_entries(id,shift_id) ON DELETE RESTRICT
);
CREATE TRIGGER shift_cash_movements_append_only BEFORE UPDATE OR DELETE ON public.shift_cash_movements
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();

CREATE FUNCTION public.ulaundry_shift_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE expected NUMERIC; latest TIMESTAMPTZ;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Shifts cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.closed_at IS NOT NULL OR EXISTS(SELECT 1 FROM public.cash_shifts WHERE cashier_id=NEW.cashier_id AND closed_at>NEW.opened_at) THEN
   RAISE EXCEPTION 'Invalid opening state or chronology' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
 END IF;
 IF OLD.closed_at IS NOT NULL OR NEW.closed_at IS NULL
 OR (NEW.id,NEW.cashier_id,NEW.open_key,NEW.opening,NEW.opening_notes,NEW.opened_at)
 IS DISTINCT FROM (OLD.id,OLD.cashier_id,OLD.open_key,OLD.opening,OLD.opening_notes,OLD.opened_at) THEN
  RAISE EXCEPTION 'Only closing an open shift is allowed' USING ERRCODE='23514';
 END IF;
 SELECT OLD.opening::numeric+COALESCE(sum(CASE WHEN method='cash' THEN CASE WHEN type='DEBIT' THEN amount::numeric ELSE -amount::numeric END ELSE 0 END),0),max(occurred_at)
 INTO expected,latest FROM public.ledger_entries WHERE shift_id=OLD.id;
 IF NEW.system_cash IS DISTINCT FROM expected OR NEW.closed_at<latest THEN
  RAISE EXCEPTION 'Invalid shift reconciliation or chronology' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cash_shift_guard BEFORE INSERT OR UPDATE OR DELETE ON public.cash_shifts
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_shift_guard();

-- All postings and close serialize on the shift row. The financial service locks
-- order first, then shift; close/movements never lock an order.
CREATE FUNCTION public.ulaundry_shift_posting_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE s public.cash_shifts%ROWTYPE; balance NUMERIC; latest TIMESTAMPTZ;
BEGIN
 IF NEW.method='cash' AND NEW.shift_id IS NULL THEN
  RAISE EXCEPTION 'Cash requires an active shift' USING ERRCODE='23514';
 END IF;
 IF NEW.shift_id IS NULL THEN RETURN NEW; END IF;
 SELECT * INTO s FROM public.cash_shifts WHERE id=NEW.shift_id FOR UPDATE;
 IF NOT FOUND OR s.closed_at IS NOT NULL OR s.cashier_id<>NEW.recorded_by OR NEW.occurred_at<s.opened_at THEN
  RAISE EXCEPTION 'Invalid posting shift' USING ERRCODE='23514';
 END IF;
 SELECT s.opening::numeric+COALESCE(sum(CASE WHEN method='cash' THEN CASE WHEN type='DEBIT' THEN amount::numeric ELSE -amount::numeric END ELSE 0 END),0),max(occurred_at)
 INTO balance,latest FROM public.ledger_entries WHERE shift_id=s.id;
 IF NEW.occurred_at<latest THEN RAISE EXCEPTION 'Posting predates last shift entry' USING ERRCODE='23514'; END IF;
 IF NEW.method='cash' THEN
  balance:=balance+CASE WHEN NEW.type='DEBIT' THEN NEW.amount::numeric ELSE -NEW.amount::numeric END;
  IF balance<0 OR balance>9223372036854775807 THEN RAISE EXCEPTION 'Invalid drawer balance' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.payment_id IS NULL AND NEW.refund_id IS NULL AND (NEW.method IS DISTINCT FROM 'cash' OR NEW.order_id IS NOT NULL) THEN
  RAISE EXCEPTION 'Shift adjustment must be cash only' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ledger_shift_posting_guard BEFORE INSERT ON public.ledger_entries
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_shift_posting_guard();
CREATE FUNCTION public.ulaundry_shift_movement_required() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF NEW.shift_id IS NOT NULL AND NEW.payment_id IS NULL AND NEW.refund_id IS NULL
 AND NOT EXISTS(SELECT 1 FROM public.shift_cash_movements WHERE ledger_id=NEW.id AND shift_id=NEW.shift_id) THEN
  RAISE EXCEPTION 'Cash movement requires an idempotency receipt' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER ledger_shift_movement_required AFTER INSERT ON public.ledger_entries
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.ulaundry_shift_movement_required();
CREATE FUNCTION public.ulaundry_cash_movement_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE s public.cash_shifts%ROWTYPE;
BEGIN
 SELECT * INTO s FROM public.cash_shifts WHERE id=NEW.shift_id FOR UPDATE;
 IF NOT FOUND OR s.closed_at IS NOT NULL OR NOT EXISTS(SELECT 1 FROM public.ledger_entries l
 WHERE l.id=NEW.ledger_id AND l.shift_id=NEW.shift_id AND l.payment_id IS NULL AND l.refund_id IS NULL AND l.method='cash') THEN
  RAISE EXCEPTION 'Invalid cash movement' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER shift_cash_movement_guard BEFORE INSERT ON public.shift_cash_movements
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_cash_movement_guard();

ALTER TABLE public.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_events_check CHECK (
 (entity='order' AND action IN('order_created','order_status_changed','order_cancelled','discount_approved','order_tracking_rotated'))
 OR(entity='payment' AND action='payment_recorded') OR(entity='refund' AND action='payment_refunded')
 OR(entity='shift' AND action IN('shift_opened','shift_closed','shift_cash_moved'))
 OR(entity='customer' AND action IN('customer_created','customer_updated'))
 OR(entity='service' AND action IN('service_configured','service_updated')) OR(entity='report' AND action='report_generated')
 OR(entity='staff' AND action IN('staff_updated','staff_logged_in','staff_created','staff_bootstrapped','staff_deactivated','staff_activated','staff_role_changed','staff_password_reset'))
);
ALTER TABLE public.auth_events DROP CONSTRAINT auth_events_permission_check;
ALTER TABLE public.auth_events ADD CONSTRAINT auth_events_permission_check CHECK(
 (kind='access_denied' AND permission IN('manage_staff','read_audit','manage_configuration','approve_discount','create_customer','configure_service','create_order','change_status','record_payment','refund_payment','cancel_order','read_orders','generate_report','manage_shift'))
 OR(kind<>'access_denied' AND permission='')
);
