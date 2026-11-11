-- Additive workflow evidence. Unknown historical processing times stay NULL.
ALTER TABLE public.orders ADD COLUMN processing_at TIMESTAMPTZ;
ALTER TABLE public.orders ADD CONSTRAINT orders_processing_time CHECK (
    processing_at IS NULL OR (isfinite(processing_at) AND status <> 'received'
    AND processing_at >= created_at AND processing_at <= updated_at
    AND (ready_at IS NULL OR processing_at <= ready_at)
    AND (cancelled_at IS NULL OR processing_at <= cancelled_at))
);

CREATE TABLE public.order_status_events (
    order_id VARCHAR(100) NOT NULL REFERENCES public.orders(id) ON DELETE RESTRICT,
    version BIGINT NOT NULL CHECK(version > 0),
    from_status VARCHAR(15),
    to_status VARCHAR(15) NOT NULL CHECK(to_status IN('received','processing','ready','collected','cancelled')),
    actor_id VARCHAR(100) REFERENCES public.staff(id) ON DELETE RESTRICT,
    occurred_at TIMESTAMPTZ NOT NULL CHECK(isfinite(occurred_at)),
    source VARCHAR(10) NOT NULL CHECK(source IN('baseline','staff')),
    PRIMARY KEY(order_id,version),
    CHECK((source='baseline' AND from_status IS NULL AND actor_id IS NULL)
      OR (source='staff' AND actor_id IS NOT NULL AND (
        (version=1 AND from_status IS NULL AND to_status='received')
        OR (version>1 AND from_status IS NOT NULL AND (
          (from_status='received' AND to_status IN('processing','cancelled'))
          OR (from_status='processing' AND to_status IN('ready','cancelled'))
          OR (from_status='ready' AND to_status='collected'))))))
);
INSERT INTO public.order_status_events(order_id,version,to_status,occurred_at,source)
SELECT id,version,status,updated_at,'baseline' FROM public.orders;
CREATE TRIGGER order_status_events_append_only BEFORE UPDATE OR DELETE ON public.order_status_events
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();

CREATE TABLE public.order_command_receipts (
    actor_id VARCHAR(100) NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(100) NOT NULL CHECK(idempotency_key ~ '^[A-Za-z0-9_.-]{1,100}$'),
    operation VARCHAR(10) NOT NULL CHECK(operation IN('create','status','cancel')),
    payload_hash BYTEA NOT NULL CHECK(octet_length(payload_hash)=32),
    order_id VARCHAR(100) NOT NULL REFERENCES public.orders(id) ON DELETE RESTRICT,
    applied_version BIGINT NOT NULL CHECK(applied_version > 0),
    PRIMARY KEY(actor_id,idempotency_key)
);
CREATE TRIGGER order_command_receipts_append_only BEFORE UPDATE OR DELETE ON public.order_command_receipts
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_audit_append_only();

-- The UPDATE row lock also serializes payment/state commands. Repeating a
-- command is handled by receipts before UPDATE, never by bypassing this graph.
CREATE FUNCTION public.ulaundry_order_workflow_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
    IF TG_OP='INSERT' THEN
        IF NEW.status<>'received' OR NEW.version<>1 OR NEW.processing_at IS NOT NULL THEN
            RAISE EXCEPTION 'Invalid initial order state' USING ERRCODE='23514';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at
        OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
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
        OR (OLD.status='processing' AND NEW.status IN('ready','cancelled'))
        OR (OLD.status='ready' AND NEW.status='collected')) THEN
        RAISE EXCEPTION 'Invalid order transition' USING ERRCODE='23514';
    END IF;
    IF (OLD.status<>'received' OR NEW.status<>'processing')
        AND NEW.processing_at IS DISTINCT FROM OLD.processing_at THEN
        RAISE EXCEPTION 'Processing milestone is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.status='processing' AND NEW.processing_at IS DISTINCT FROM NEW.updated_at THEN
        RAISE EXCEPTION 'Processing milestone required' USING ERRCODE='23514';
    END IF;
    IF NEW.status='ready' AND NEW.ready_at IS DISTINCT FROM NEW.updated_at THEN
        RAISE EXCEPTION 'Ready milestone required' USING ERRCODE='23514';
    END IF;
    IF NEW.status='collected' AND (NEW.collected_at IS DISTINCT FROM NEW.updated_at
        OR NEW.ready_at IS DISTINCT FROM OLD.ready_at
        OR (SELECT COALESCE(sum(amount),0) FROM public.payments WHERE order_id=OLD.id)<>NEW.total) THEN
        RAISE EXCEPTION 'Paid order and pickup milestone required' USING ERRCODE='23514';
    END IF;
    IF NEW.status='cancelled' AND (NEW.cancelled_at IS DISTINCT FROM NEW.updated_at
        OR EXISTS(SELECT 1 FROM public.payments WHERE order_id=OLD.id)) THEN
        RAISE EXCEPTION 'Unpaid order and cancellation milestone required' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER orders_workflow_guard BEFORE INSERT OR UPDATE ON public.orders
FOR EACH ROW EXECUTE FUNCTION public.ulaundry_order_workflow_guard();
