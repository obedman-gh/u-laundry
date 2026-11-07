-- Additive storage for the day-05 domain. Legacy pesanan/kasir/keuangan and
-- their history remain untouched. No automatic status/user/money conversion.
CREATE TABLE public.customers (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) > 0),
    contact VARCHAR(16) NOT NULL DEFAULT '',
    notes VARCHAR(1000) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);

CREATE TABLE public.services (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) > 0),
    unit VARCHAR(10) NOT NULL CHECK (unit IN ('kg','piece')),
    unit_price BIGINT NOT NULL CHECK (unit_price >= 0),
    turnaround_ns BIGINT NOT NULL CHECK (turnaround_ns > 0),
    active BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
CREATE INDEX services_active_name_idx ON public.services (name,id) WHERE active;

CREATE TABLE public.staff (
    id VARCHAR(100) PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE CHECK (length(username) >= 3),
    name VARCHAR(100) NOT NULL CHECK (length(btrim(name)) > 0),
    role VARCHAR(10) NOT NULL CHECK (role IN ('owner','admin','cashier')),
    active BOOLEAN NOT NULL,
    password_hash TEXT NOT NULL CHECK (length(password_hash) > 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);

CREATE TABLE public.orders (
    id VARCHAR(100) PRIMARY KEY,
    customer_id VARCHAR(100) NOT NULL REFERENCES public.customers(id) ON DELETE RESTRICT,
    customer_name VARCHAR(100) NOT NULL,
    customer_contact VARCHAR(16) NOT NULL,
    customer_notes VARCHAR(1000) NOT NULL,
    customer_created_at TIMESTAMPTZ NOT NULL,
    customer_updated_at TIMESTAMPTZ NOT NULL CHECK (customer_updated_at >= customer_created_at),
    created_by VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
    item_count INTEGER NOT NULL CHECK (item_count BETWEEN 1 AND 100),
    subtotal BIGINT NOT NULL CHECK (subtotal >= 0),
    discount BIGINT NOT NULL CHECK (discount >= 0 AND discount <= subtotal),
    discount_reason VARCHAR(250) NOT NULL,
    discount_approved_by VARCHAR(100) REFERENCES public.staff(id) ON DELETE RESTRICT,
    total BIGINT NOT NULL CHECK (total >= 0 AND total = subtotal - discount),
    notes VARCHAR(1000) NOT NULL,
    status VARCHAR(15) NOT NULL CHECK (status IN ('received','processing','ready','collected','cancelled')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    estimated_ready_at TIMESTAMPTZ NOT NULL CHECK (estimated_ready_at > created_at),
    ready_at TIMESTAMPTZ CHECK (ready_at >= created_at AND ready_at <= updated_at),
    collected_at TIMESTAMPTZ CHECK (collected_at >= ready_at AND collected_at <= updated_at),
    cancelled_at TIMESTAMPTZ CHECK (cancelled_at >= created_at AND cancelled_at <= updated_at),
    cancellation_reason VARCHAR(250) NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    CHECK ((discount = 0 AND discount_reason = '' AND discount_approved_by IS NULL)
        OR (discount > 0 AND length(btrim(discount_reason)) > 0 AND discount_approved_by IS NOT NULL)),
    CHECK ((status IN ('received','processing') AND ready_at IS NULL AND collected_at IS NULL AND cancelled_at IS NULL)
        OR (status = 'ready' AND ready_at IS NOT NULL AND collected_at IS NULL AND cancelled_at IS NULL)
        OR (status = 'collected' AND ready_at IS NOT NULL AND collected_at IS NOT NULL AND cancelled_at IS NULL)
        OR (status = 'cancelled' AND ready_at IS NULL AND collected_at IS NULL AND cancelled_at IS NOT NULL)),
    CHECK ((status = 'cancelled' AND length(btrim(cancellation_reason)) > 0)
        OR (status <> 'cancelled' AND cancellation_reason = ''))
);
CREATE INDEX orders_created_idx ON public.orders (created_at DESC,id DESC);
CREATE INDEX orders_status_created_idx ON public.orders (status,created_at DESC,id DESC);
CREATE INDEX orders_customer_created_idx ON public.orders (customer_id,created_at DESC,id DESC);

CREATE TABLE public.order_items (
    order_id VARCHAR(100) NOT NULL REFERENCES public.orders(id) ON DELETE RESTRICT,
    line_no INTEGER NOT NULL CHECK (line_no BETWEEN 1 AND 100),
    service_id VARCHAR(100) NOT NULL REFERENCES public.services(id) ON DELETE RESTRICT,
    service_name VARCHAR(100) NOT NULL,
    unit VARCHAR(10) NOT NULL CHECK (unit IN ('kg','piece')),
    unit_price BIGINT NOT NULL CHECK (unit_price >= 0),
    pieces BIGINT NOT NULL,
    grams BIGINT NOT NULL,
    line_total BIGINT NOT NULL CHECK (line_total >= 0),
    PRIMARY KEY (order_id,line_no),
    CHECK ((unit='piece' AND pieces > 0 AND grams=0 AND line_total=unit_price::numeric*pieces)
        OR (unit='kg' AND grams > 0 AND pieces=0 AND line_total=floor((unit_price::numeric*grams+500)/1000)))
);
CREATE INDEX order_items_service_idx ON public.order_items(service_id);

CREATE TABLE public.payments (
    id VARCHAR(100) PRIMARY KEY,
    order_id VARCHAR(100) NOT NULL REFERENCES public.orders(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(100) NOT NULL CHECK (length(idempotency_key) > 0),
    amount BIGINT NOT NULL CHECK (amount > 0),
    method VARCHAR(15) NOT NULL CHECK (method IN ('cash','bank_transfer','qris')),
    recorded_by VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
    received_at TIMESTAMPTZ NOT NULL,
    UNIQUE (order_id,idempotency_key),
    UNIQUE (id,order_id,amount)
);
CREATE INDEX payments_order_time_idx ON public.payments(order_id,received_at,id);

CREATE TABLE public.ledger_entries (
    id VARCHAR(100) PRIMARY KEY,
    order_id VARCHAR(100) REFERENCES public.orders(id) ON DELETE RESTRICT,
    payment_id VARCHAR(100) UNIQUE,
    type VARCHAR(10) NOT NULL CHECK (type IN ('DEBIT','KREDIT')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    description VARCHAR(250) NOT NULL CHECK (length(btrim(description)) > 0),
    recorded_by VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
    occurred_at TIMESTAMPTZ NOT NULL,
    CHECK (payment_id IS NULL OR (order_id IS NOT NULL AND type='DEBIT')),
    FOREIGN KEY (payment_id,order_id,amount) REFERENCES public.payments(id,order_id,amount) ON DELETE RESTRICT
);
CREATE INDEX ledger_order_time_idx ON public.ledger_entries(order_id,occurred_at,id);

CREATE TABLE public.audit_events (
    id VARCHAR(100) PRIMARY KEY,
    actor_id VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
    action VARCHAR(30) NOT NULL,
    entity VARCHAR(15) NOT NULL,
    entity_id VARCHAR(100) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    CHECK ((entity='order' AND action IN ('order_created','order_status_changed','discount_approved'))
        OR (entity='payment' AND action='payment_recorded')
        OR (entity='customer' AND action='customer_updated')
        OR (entity='service' AND action='service_updated')
        OR (entity='staff' AND action='staff_updated'))
);
CREATE INDEX audit_entity_time_idx ON public.audit_events(entity,entity_id,occurred_at,id);

CREATE FUNCTION public.ulaundry_audit_append_only() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    RAISE EXCEPTION 'Audit events are append only' USING ERRCODE='23514';
END $$;
CREATE TRIGGER audit_events_append_only BEFORE UPDATE OR DELETE ON public.audit_events
    FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();
