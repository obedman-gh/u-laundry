-- Manual payment receipts and full reversals. No historical cash is invented.
ALTER TABLE public.payments ADD COLUMN status VARCHAR(10) NOT NULL DEFAULT 'posted'
    CHECK(status='posted');
ALTER TABLE public.payments ADD CONSTRAINT payments_key_format
    CHECK(idempotency_key ~ '^[A-Za-z0-9_.-]{1,100}$');
ALTER TABLE public.payments ADD CONSTRAINT payments_time_finite CHECK(isfinite(received_at));
ALTER TABLE public.payments ADD CONSTRAINT payments_refund_reference UNIQUE(id,order_id,amount,method);

CREATE TABLE public.refunds (
    id VARCHAR(100) PRIMARY KEY,
    order_id VARCHAR(100) NOT NULL REFERENCES public.orders(id) ON DELETE RESTRICT,
    payment_id VARCHAR(100) NOT NULL UNIQUE,
    idempotency_key VARCHAR(100) NOT NULL CHECK(idempotency_key ~ '^[A-Za-z0-9_.-]{1,100}$'),
    amount BIGINT NOT NULL CHECK(amount>0),
    method VARCHAR(15) NOT NULL CHECK(method IN('cash','bank_transfer','qris')),
    recorded_by VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
    refunded_at TIMESTAMPTZ NOT NULL CHECK(isfinite(refunded_at)),
    reason VARCHAR(250) NOT NULL CHECK(length(btrim(reason))>0),
    UNIQUE(order_id,idempotency_key),
    UNIQUE(id,order_id,amount),
    FOREIGN KEY(payment_id,order_id,amount,method) REFERENCES public.payments(id,order_id,amount,method) ON DELETE RESTRICT
);
CREATE INDEX refunds_order_time_idx ON public.refunds(order_id,refunded_at,id);

ALTER TABLE public.ledger_entries ADD COLUMN refund_id VARCHAR(100) UNIQUE;
ALTER TABLE public.ledger_entries ADD COLUMN method VARCHAR(15) CHECK(method IN('cash','bank_transfer','qris'));
UPDATE public.ledger_entries l SET method=p.method FROM public.payments p WHERE l.payment_id=p.id;
ALTER TABLE public.ledger_entries ADD CONSTRAINT ledger_refund_reference
    FOREIGN KEY(refund_id,order_id,amount) REFERENCES public.refunds(id,order_id,amount) ON DELETE RESTRICT;
ALTER TABLE public.ledger_entries ADD CONSTRAINT ledger_financial_reference CHECK(
    (refund_id IS NULL OR (payment_id IS NULL AND order_id IS NOT NULL AND type='KREDIT'))
    AND ((payment_id IS NULL AND refund_id IS NULL) OR method IS NOT NULL)
);
ALTER TABLE public.ledger_entries ADD CONSTRAINT ledger_time_finite CHECK(isfinite(occurred_at));

DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM public.payments p LEFT JOIN public.ledger_entries l ON l.payment_id=p.id
        WHERE l.id IS NULL OR (l.order_id,l.amount,l.type,l.recorded_by,l.occurred_at)
        IS DISTINCT FROM (p.order_id,p.amount,'DEBIT'::varchar,p.recorded_by,p.received_at)) THEN
        RAISE EXCEPTION 'Existing payment ledger requires reconciliation' USING ERRCODE='23514';
    END IF;
END $$;

CREATE TRIGGER payments_append_only BEFORE UPDATE OR DELETE ON public.payments
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();
CREATE TRIGGER refunds_append_only BEFORE UPDATE OR DELETE ON public.refunds
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();
CREATE TRIGGER ledger_entries_append_only BEFORE UPDATE OR DELETE ON public.ledger_entries
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();

-- Serializes even direct inserts against state changes and competing postings.
CREATE FUNCTION public.ulaundry_financial_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE o public.orders%ROWTYPE; net NUMERIC;
BEGIN
    SELECT * INTO o FROM public.orders WHERE id=NEW.order_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'Order missing' USING ERRCODE='23503'; END IF;
    IF o.status IN('cancelled','collected') THEN
        RAISE EXCEPTION 'Order closed' USING ERRCODE='23514';
    END IF;
    IF TG_TABLE_NAME='payments' THEN
        SELECT COALESCE(sum(p.amount),0) INTO net FROM public.payments p
        WHERE p.order_id=o.id AND NOT EXISTS(SELECT 1 FROM public.refunds r WHERE r.payment_id=p.id);
        IF NEW.amount>o.total-net OR NEW.received_at<o.updated_at THEN
            RAISE EXCEPTION 'Invalid payment balance or chronology' USING ERRCODE='23514';
        END IF;
    ELSE
        IF NEW.refunded_at<o.updated_at OR NOT EXISTS(SELECT 1 FROM public.payments p
            WHERE p.id=NEW.payment_id AND p.order_id=o.id AND p.received_at<=NEW.refunded_at) THEN
            RAISE EXCEPTION 'Invalid refund reference or chronology' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER payment_posting_guard BEFORE INSERT ON public.payments
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_financial_guard();
CREATE TRIGGER refund_posting_guard BEFORE INSERT ON public.refunds
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_financial_guard();

-- A receipt/reversal cannot COMMIT without exactly one matching posting.
CREATE FUNCTION public.ulaundry_financial_posting_required() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
DECLARE matches INTEGER;
BEGIN
    IF TG_TABLE_NAME='payments' THEN
        SELECT count(*) INTO matches FROM public.ledger_entries l WHERE l.payment_id=NEW.id
        AND (l.order_id,l.amount,l.type,l.method,l.recorded_by,l.occurred_at)
        IS NOT DISTINCT FROM (NEW.order_id,NEW.amount,'DEBIT'::varchar,NEW.method,NEW.recorded_by,NEW.received_at);
    ELSE
        SELECT count(*) INTO matches FROM public.ledger_entries l WHERE l.refund_id=NEW.id
        AND (l.order_id,l.amount,l.type,l.method,l.recorded_by,l.occurred_at)
        IS NOT DISTINCT FROM (NEW.order_id,NEW.amount,'KREDIT'::varchar,NEW.method,NEW.recorded_by,NEW.refunded_at);
    END IF;
    IF matches<>1 THEN RAISE EXCEPTION 'Matching financial posting required' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER payment_posting_required AFTER INSERT ON public.payments
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.ulaundry_financial_posting_required();
CREATE CONSTRAINT TRIGGER refund_posting_required AFTER INSERT ON public.refunds
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.ulaundry_financial_posting_required();

ALTER TABLE public.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_events_check CHECK (
 (entity='order' AND action IN('order_created','order_status_changed','order_cancelled','discount_approved','order_tracking_rotated'))
 OR(entity='payment' AND action='payment_recorded') OR(entity='refund' AND action='payment_refunded')
 OR(entity='customer' AND action IN('customer_created','customer_updated'))
 OR(entity='service' AND action IN('service_configured','service_updated')) OR(entity='report' AND action='report_generated')
 OR(entity='staff' AND action IN('staff_updated','staff_logged_in','staff_created','staff_bootstrapped','staff_deactivated','staff_activated','staff_role_changed','staff_password_reset'))
);
ALTER TABLE public.auth_events DROP CONSTRAINT auth_events_permission_check;
ALTER TABLE public.auth_events ADD CONSTRAINT auth_events_permission_check CHECK(
 (kind='access_denied' AND permission IN('manage_staff','read_audit','manage_configuration','approve_discount','create_customer','configure_service','create_order','change_status','record_payment','refund_payment','cancel_order','read_orders','generate_report'))
 OR(kind<>'access_denied' AND permission='')
);

-- Same graph/milestone rules as version 9; balances now exclude full refunds.
CREATE OR REPLACE FUNCTION public.ulaundry_order_workflow_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF NEW.status<>'received' OR NEW.version<>1 OR NEW.processing_at IS NOT NULL THEN
            RAISE EXCEPTION 'Invalid initial order state' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'Invalid order version or chronology' USING ERRCODE='23514';
    END IF;
    IF NEW.status=OLD.status THEN
        IF (NEW.processing_at,NEW.ready_at,NEW.collected_at,NEW.cancelled_at,NEW.cancellation_reason)
           IS DISTINCT FROM (OLD.processing_at,OLD.ready_at,OLD.collected_at,OLD.cancelled_at,OLD.cancellation_reason) THEN
            RAISE EXCEPTION 'Order milestones are immutable' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NOT ((OLD.status='received' AND NEW.status IN('processing','cancelled'))
        OR (OLD.status='processing' AND NEW.status IN('ready','cancelled')) OR (OLD.status='ready' AND NEW.status='collected')) THEN
        RAISE EXCEPTION 'Invalid order transition' USING ERRCODE='23514';
    END IF;
    IF (OLD.status<>'received' OR NEW.status<>'processing') AND NEW.processing_at IS DISTINCT FROM OLD.processing_at THEN
        RAISE EXCEPTION 'Processing milestone is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.status='processing' AND NEW.processing_at IS DISTINCT FROM NEW.updated_at THEN
        RAISE EXCEPTION 'Processing milestone required' USING ERRCODE='23514';
    END IF;
    IF NEW.status='ready' AND NEW.ready_at IS DISTINCT FROM NEW.updated_at THEN
        RAISE EXCEPTION 'Ready milestone required' USING ERRCODE='23514';
    END IF;
    IF NEW.status='collected' AND (NEW.collected_at IS DISTINCT FROM NEW.updated_at OR NEW.ready_at IS DISTINCT FROM OLD.ready_at
        OR (SELECT COALESCE(sum(p.amount),0) FROM public.payments p WHERE p.order_id=OLD.id
            AND NOT EXISTS(SELECT 1 FROM public.refunds r WHERE r.payment_id=p.id))<>NEW.total) THEN
        RAISE EXCEPTION 'Paid order and pickup milestone required' USING ERRCODE='23514';
    END IF;
    IF NEW.status='cancelled' AND (NEW.cancelled_at IS DISTINCT FROM NEW.updated_at
        OR EXISTS(SELECT 1 FROM public.payments p WHERE p.order_id=OLD.id
            AND NOT EXISTS(SELECT 1 FROM public.refunds r WHERE r.payment_id=p.id))) THEN
        RAISE EXCEPTION 'Unpaid order and cancellation milestone required' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
