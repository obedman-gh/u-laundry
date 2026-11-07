-- Additive workflow support; no legacy data/status/account conversion.
ALTER TABLE public.services ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0);
CREATE TABLE public.order_tracking_tokens (
    order_id VARCHAR(100) PRIMARY KEY REFERENCES public.orders(id) ON DELETE RESTRICT,
    token_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash)=32)
);
-- Expand allowed audit actions without changing existing events.
ALTER TABLE public.audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE public.audit_events ADD CONSTRAINT audit_events_check CHECK (
    (entity='order' AND action IN ('order_created','order_status_changed','order_cancelled','discount_approved'))
    OR (entity='payment' AND action='payment_recorded')
    OR (entity='customer' AND action IN ('customer_created','customer_updated'))
    OR (entity='service' AND action IN ('service_configured','service_updated'))
    OR (entity='staff' AND action IN ('staff_updated','staff_logged_in'))
    OR (entity='report' AND action='report_generated')
);
CREATE INDEX ledger_occurred_type_idx ON public.ledger_entries(occurred_at,type) INCLUDE(amount);
